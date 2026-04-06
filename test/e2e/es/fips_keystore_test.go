// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonkeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	esfips "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/fips"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

const (
	fipsSetting = "xpack.security.fips_mode.enabled"
)

func TestFIPSKeystoreManagedResources(t *testing.T) {
	k := test.NewK8sClientOrFatal()
	b := elasticsearch.NewBuilder("test-fips-keystore-managed-resources").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithAdditionalConfig(map[string]map[string]any{
			"masterdata": {
				fipsSetting: true,
			},
		})

	steps := test.StepList{}.
		WithSteps(b.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithSteps(b.CreationTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretCreatedStep(k, b.Elasticsearch)).
		WithStep(checkFIPSPodTemplateInjectedStep(k, b.Elasticsearch, false)).
		WithSteps(b.DeletionTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretDeletedStep(k, b.Elasticsearch))

	steps.RunSequential(t)
}

func TestFIPSKeystoreUserOverrideSkipsManagement(t *testing.T) {
	k := test.NewK8sClientOrFatal()
	const userPassphraseFile = "/tmp/user-managed-fips-passphrase"
	b := elasticsearch.NewBuilder("test-fips-keystore-user-override").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithAdditionalConfig(map[string]map[string]any{
			"masterdata": {
				fipsSetting: true,
			},
		}).
		WithEnvironmentVariable("KEYSTORE_PASSWORD_FILE", userPassphraseFile)

	steps := test.StepList{}.
		WithSteps(b.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithSteps(b.CreationTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretAbsentStep(k, b.Elasticsearch)).
		WithStep(checkFIPSPodTemplateInjectedStep(k, b.Elasticsearch, true)).
		WithSteps(b.DeletionTestSteps(k))

	steps.RunSequential(t)
}

func TestFIPSKeystoreSecretDeletedWhenFIPSDisabled(t *testing.T) {
	k := test.NewK8sClientOrFatal()
	b := elasticsearch.NewBuilder("test-fips-keystore-disable-cleans-secret").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithAdditionalConfig(map[string]map[string]any{
			"masterdata": {
				fipsSetting: true,
			},
		})

	steps := test.StepList{}.
		WithSteps(b.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithSteps(b.CreationTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretCreatedStep(k, b.Elasticsearch)).
		WithStep(test.Step{
			Name: "Disable FIPS mode in Elasticsearch spec",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &es); err != nil {
					return err
				}
				if len(es.Spec.NodeSets) == 0 {
					return fmt.Errorf("expected at least one nodeset")
				}
				if es.Spec.NodeSets[0].Config == nil {
					es.Spec.NodeSets[0].Config = b.Elasticsearch.Spec.NodeSets[0].Config.DeepCopy()
				}
				es.Spec.NodeSets[0].Config.Data[fipsSetting] = false
				return k.Client.Update(context.Background(), &es)
			}),
		}).
		WithStep(checkFIPSKeystoreSecretDeletedStep(k, b.Elasticsearch)).
		WithSteps(b.DeletionTestSteps(k))

	steps.RunSequential(t)
}

func deleteFIPSKeystoreSecretStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return test.Step{
		Name: "Delete stale FIPS keystore password secret if present",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			secret.Namespace = es.Namespace
			secret.Name = esv1.FIPSKeystorePasswordSecret(es.Name)
			err := k.Client.Delete(context.Background(), &secret)
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}),
	}
}

func checkFIPSKeystoreSecretCreatedStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return test.Step{
		Name: "FIPS keystore password secret should eventually be created",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.FIPSKeystorePasswordSecret(es.Name)}, &secret); err != nil {
				return err
			}
			passwordBytes, exists := secret.Data[esfips.KeystorePasswordKey]
			if !exists {
				return fmt.Errorf("missing key %q in FIPS secret", esfips.KeystorePasswordKey)
			}
			if len(passwordBytes) != 24 {
				return fmt.Errorf("expected 24-char generated password, got %d", len(passwordBytes))
			}
			for _, c := range passwordBytes {
				if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
					return fmt.Errorf("generated password contains non-alphanumeric byte: %q", c)
				}
			}
			return nil
		}),
	}
}

func checkFIPSKeystoreSecretAbsentStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return test.Step{
		Name: "FIPS keystore password secret should not be created",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.FIPSKeystorePasswordSecret(es.Name)}, &secret)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("unexpected secret %s/%s exists", secret.Namespace, secret.Name)
		}),
	}
}

func checkFIPSKeystoreSecretDeletedStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return test.Step{
		Name: "FIPS keystore password secret should eventually be deleted",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.FIPSKeystorePasswordSecret(es.Name)}, &secret)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("secret %s/%s still exists", secret.Namespace, secret.Name)
		}),
	}
}

func checkFIPSPodTemplateInjectedStep(k *test.K8sClient, es esv1.Elasticsearch, expectUserOverride bool) test.Step {
	stepName := "StatefulSet pod template should include FIPS keystore wiring"
	if expectUserOverride {
		stepName = "StatefulSet pod template should not include operator-managed FIPS wiring when user override is set"
	}
	return test.Step{
		Name: stepName,
		Test: test.Eventually(func() error {
			if len(es.Spec.NodeSets) == 0 {
				return fmt.Errorf("expected at least one nodeset")
			}
			var sset appsv1.StatefulSet
			if err := k.Client.Get(
				context.Background(),
				types.NamespacedName{Namespace: es.Namespace, Name: esv1.StatefulSet(es.Name, es.Spec.NodeSets[0].Name)},
				&sset,
			); err != nil {
				return err
			}

			esContainer, err := findContainerByName(sset.Spec.Template.Spec.Containers, esv1.ElasticsearchContainerName)
			if err != nil {
				return err
			}

			if expectUserOverride {
				return checkFIPSPodTemplateUserOverride(&sset.Spec.Template.Spec, esContainer)
			}
			return checkFIPSPodTemplateOperatorManaged(&sset.Spec.Template.Spec, esContainer)
		}),
	}
}

func checkFIPSPodTemplateUserOverride(pod *corev1.PodSpec, esContainer *corev1.Container) error {
	hasFIPSVolume := slices.ContainsFunc(pod.Volumes, func(v corev1.Volume) bool {
		return v.Name == esfips.VolumeName
	})
	mainHasFIPSMount := slices.ContainsFunc(esContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == esfips.VolumeName && vm.MountPath == esfips.MountPath
	})
	if hasFIPSVolume || mainHasFIPSMount {
		return fmt.Errorf("operator-managed FIPS wiring unexpectedly present in pod template")
	}

	if keystoreInit := findContainerByNameOptional(pod.InitContainers, commonkeystore.InitContainerName); keystoreInit != nil {
		initHasFIPSMount := slices.ContainsFunc(keystoreInit.VolumeMounts, func(vm corev1.VolumeMount) bool {
			return vm.Name == esfips.VolumeName && vm.MountPath == esfips.MountPath
		})
		if initHasFIPSMount || keystoreInitScriptLooksFIPS(keystoreInit.Command) {
			return fmt.Errorf("operator-managed FIPS wiring unexpectedly present in pod template")
		}
	}

	if !slices.ContainsFunc(esContainer.Env, func(e corev1.EnvVar) bool { return e.Name == "KEYSTORE_PASSWORD_FILE" }) {
		return fmt.Errorf("expected user-provided KEYSTORE_PASSWORD_FILE env var to be present")
	}
	return nil
}

func checkFIPSPodTemplateOperatorManaged(pod *corev1.PodSpec, esContainer *corev1.Container) error {
	hasFIPSVolume := slices.ContainsFunc(pod.Volumes, func(v corev1.Volume) bool {
		return v.Name == esfips.VolumeName
	})
	mainHasFIPSMount := slices.ContainsFunc(esContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == esfips.VolumeName && vm.MountPath == esfips.MountPath
	})

	keystoreInitContainer, err := findContainerByName(pod.InitContainers, commonkeystore.InitContainerName)
	if err != nil {
		return err
	}
	initHasFIPSMount := slices.ContainsFunc(keystoreInitContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == esfips.VolumeName && vm.MountPath == esfips.MountPath
	})
	mainHasPassphraseEnv := slices.ContainsFunc(esContainer.Env, func(e corev1.EnvVar) bool {
		return e.Name == "KEYSTORE_PASSWORD_FILE" &&
			e.Value == esfips.PasswordFile
	})
	mainHasPasswordEnvFromSecret := slices.ContainsFunc(esContainer.Env, func(e corev1.EnvVar) bool {
		return e.Name == "KEYSTORE_PASSWORD" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil
	})
	script := strings.Join(keystoreInitContainer.Command, " ")

	if !hasFIPSVolume {
		return fmt.Errorf("missing %s volume", esfips.VolumeName)
	}
	if !mainHasFIPSMount {
		return fmt.Errorf("missing %s mount on Elasticsearch container", esfips.MountPath)
	}
	if !initHasFIPSMount {
		return fmt.Errorf("missing %s mount on init container", esfips.MountPath)
	}
	if !mainHasPassphraseEnv {
		return fmt.Errorf("missing KEYSTORE_PASSWORD_FILE=%s env var", esfips.PasswordFile)
	}
	if mainHasPasswordEnvFromSecret {
		return fmt.Errorf("unexpected KEYSTORE_PASSWORD secret env var on Elasticsearch container")
	}
	if !keystoreInitScriptLooksFIPS(keystoreInitContainer.Command) {
		return fmt.Errorf("expected FIPS-aware keystore init script, got command %q", script)
	}
	return nil
}

// keystoreInitScriptLooksFIPS reports whether the keystore init container command matches the
// operator's FIPS-aware elasticsearch-keystore bootstrap script.
func keystoreInitScriptLooksFIPS(cmd []string) bool {
	script := strings.Join(cmd, " ")
	return strings.Contains(script, "create -p") &&
		strings.Contains(script, `KEYSTORE_PASSWORD=$(cat "`)
}

func findContainerByNameOptional(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

func findContainerByName(containers []corev1.Container, name string) (*corev1.Container, error) {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i], nil
		}
	}
	return nil, fmt.Errorf("container %q not found", name)
}
