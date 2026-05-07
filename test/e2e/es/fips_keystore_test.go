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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonkeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	eskeystorepassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorepassword"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

const (
	fipsSetting = "xpack.security.fips_mode.enabled"
)

// skipUnlessFIPSKeystoreStackVersion skips when the E2E stack version is below the operator's
// FIPS managed-keystore threshold (same semver rule as reconcileManagedKeystorePasswordSecret).
func skipUnlessFIPSKeystoreStackVersion(t *testing.T) {
	t.Helper()
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(esversion.KeystorePasswordMinVersion) {
		t.Skipf("Skipping test: stack version %s is below FIPS managed keystore minimum %s",
			test.Ctx().ElasticStackVersion, esversion.KeystorePasswordMinVersion.String())
	}
}

// skipUnlessStackConfigPolicySupported skips when StackConfigPolicy is not available
// in the current E2E environment.
func skipUnlessStackConfigPolicySupported(t *testing.T) {
	t.Helper()
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping test: stack config policy requires a test license")
	}

	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(filesettings.FileBasedSettingsMinPreVersion) {
		t.Skipf("Skipping test: stack version %s is below StackConfigPolicy minimum %s",
			test.Ctx().ElasticStackVersion, filesettings.FileBasedSettingsMinPreVersion.String())
	}
}

func TestFIPSKeystoreManagedResources(t *testing.T) {
	skipUnlessFIPSKeystoreStackVersion(t)
	k := test.NewK8sClientOrFatal()
	b := elasticsearch.NewBuilder("test-fips-ks-managed").
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
		WithSteps(test.CheckTestSteps(b, k)).
		WithStep(checkFIPSKeystoreSecretCreatedStep(k, b.Elasticsearch)).
		WithStep(checkFIPSPodTemplateInjectedStep(k, b.Elasticsearch, false)).
		WithSteps(b.DeletionTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretDeletedStep(k, b.Elasticsearch))

	steps.RunSequential(t)
}

func TestFIPSKeystoreUserOverrideSkipsManagement(t *testing.T) {
	skipUnlessFIPSKeystoreStackVersion(t)
	k := test.NewK8sClientOrFatal()

	const (
		passphraseSecretKey  = "passphrase"
		passphraseMountPath  = "/mnt/elastic-internal/e2e-user-fips-keystore-passphrase"
		passphraseVolumeName = "e2e-user-fips-keystore-passphrase"
	)
	passphraseSecretName := fmt.Sprintf("test-fips-ks-user-override-pass-%s", rand.String(4))
	userPassphraseFile := fmt.Sprintf("%s/%s", passphraseMountPath, passphraseSecretKey)

	b := elasticsearch.NewBuilder("test-fips-ks-user-override").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithAdditionalConfig(map[string]map[string]any{
			"masterdata": {
				fipsSetting: true,
			},
		}).
		WithSecretVolumeMountForElasticsearch(passphraseVolumeName, passphraseSecretName, passphraseMountPath).
		WithEnvironmentVariable("KEYSTORE_PASSWORD_FILE", userPassphraseFile)

	steps := test.StepList{}.
		WithStep(test.Step{
			Name: "Create user FIPS keystore passphrase secret",
			Test: test.Eventually(func() error {
				sec := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      passphraseSecretName,
						Namespace: b.Elasticsearch.Namespace,
					},
					Data: map[string][]byte{
						// Fixed-length passphrase for the user-managed keystore file (not operator-generated).
						passphraseSecretKey: []byte("e2e-user-fips-kst-phr-32bytes!!"),
					},
				}
				return k.CreateOrUpdate(&sec)
			}),
		}).
		WithSteps(b.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithStep(checkFIPSKeystoreSecretAbsentStep(k, b.Elasticsearch)).
		WithStep(checkFIPSPodTemplateInjectedStep(k, b.Elasticsearch, true)).
		WithSteps(b.DeletionTestSteps(k)).
		WithStep(test.Step{
			Name: fmt.Sprintf("Delete secret %s/%s", b.Elasticsearch.Namespace, passphraseSecretName),
			Test: test.Eventually(func() error {
				var sec corev1.Secret
				sec.Namespace = b.Elasticsearch.Namespace
				sec.Name = passphraseSecretName
				err := k.Client.Delete(context.Background(), &sec)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		})

	steps.RunSequential(t)
}

func TestFIPSKeystoreSecretDeletedWhenFIPSDisabled(t *testing.T) {
	skipUnlessFIPSKeystoreStackVersion(t)
	k := test.NewK8sClientOrFatal()
	b := elasticsearch.NewBuilder("test-fips-ks-off-cleans-secret").
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
		WithSteps(test.CheckTestSteps(b, k)).
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
		WithStep(elasticsearch.CheckClusterHealth(b, k)).
		WithSteps(b.DeletionTestSteps(k))

	steps.RunSequential(t)
}

// TestFIPSKeystoreManagedResourcesFromStackConfigPolicy validates managed FIPS
// keystore resources when FIPS mode is enabled by StackConfigPolicy config.
func TestFIPSKeystoreManagedResourcesFromStackConfigPolicy(t *testing.T) {
	skipUnlessFIPSKeystoreStackVersion(t)
	skipUnlessStackConfigPolicySupported(t)

	k := test.NewK8sClientOrFatal()
	selectorValue := rand.String(4)
	b := elasticsearch.NewBuilder("test-fips-ks-scp").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithLabel("fips-ks-scp", selectorValue)
	bWithLicense := test.LicenseTestBuilder(b)

	config := commonv1.NewConfig(map[string]any{
		fipsSetting: true,
	})
	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.Elasticsearch.Namespace,
			Name:      fmt.Sprintf("test-fips-ks-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"fips-ks-scp": selectorValue},
			},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				Config: &config,
			},
		},
	}

	steps := test.StepList{}.
		WithSteps(bWithLicense.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithStep(test.Step{
			Name: "Create StackConfigPolicy with FIPS mode enabled",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(&policy)
			}),
		}).
		WithSteps(bWithLicense.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(bWithLicense, k)).
		WithStep(checkFIPSKeystoreSecretCreatedStep(k, b.Elasticsearch)).
		WithStep(checkFIPSPodTemplateInjectedStep(k, b.Elasticsearch, false)).
		WithStep(test.Step{
			Name: fmt.Sprintf("Delete StackConfigPolicy %s/%s", policy.Namespace, policy.Name),
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &policy)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		}).
		WithSteps(bWithLicense.DeletionTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretDeletedStep(k, b.Elasticsearch))

	steps.RunSequential(t)
}

// TestFIPSKeystoreWithSecureSettings validates that secure settings are added
// through the password-protected keystore flow when FIPS mode is enabled.
func TestFIPSKeystoreWithSecureSettings(t *testing.T) {
	skipUnlessFIPSKeystoreStackVersion(t)

	k := test.NewK8sClientOrFatal()
	b := elasticsearch.NewBuilder("test-fips-ks-secure-settings").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithAdditionalConfig(map[string]map[string]any{
			"masterdata": {
				fipsSetting: true,
			},
		})

	const secureSettingKey = "xpack.notification.email.account.foo.smtp.secure_password"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("test-fips-ks-secure-settings-%s", rand.String(4)),
			Namespace: b.Elasticsearch.Namespace,
		},
		Data: map[string][]byte{
			secureSettingKey: []byte("foo_pw"),
		},
	}
	b = b.WithESSecureSettings(secureSettings.Name)

	steps := test.StepList{}.
		WithStep(test.Step{
			Name: "Create secure settings secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(&secureSettings)
			}),
		}).
		WithSteps(b.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithStep(checkFIPSKeystoreSecretCreatedStep(k, b.Elasticsearch)).
		WithStep(checkFIPSPodTemplateInjectedStep(k, b.Elasticsearch, false)).
		WithStep(elasticsearch.CheckESKeystoreEntries(k, b, []string{
			secureSettingKey,
		})).
		WithSteps(b.DeletionTestSteps(k)).
		WithStep(checkFIPSKeystoreSecretDeletedStep(k, b.Elasticsearch)).
		WithStep(test.Step{
			Name: fmt.Sprintf("Delete secret %s/%s", secureSettings.Namespace, secureSettings.Name),
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &secureSettings)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		})

	steps.RunSequential(t)
}

func deleteFIPSKeystoreSecretStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return test.Step{
		Name: "Delete stale FIPS keystore password secret if present",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			secret.Namespace = es.Namespace
			secret.Name = esv1.KeystorePasswordSecret(es.Name)
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
			if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.KeystorePasswordSecret(es.Name)}, &secret); err != nil {
				return err
			}
			passwordBytes, exists := secret.Data[eskeystorepassword.KeystorePasswordKey]
			if !exists {
				return fmt.Errorf("missing key %q in keystore password secret", eskeystorepassword.KeystorePasswordKey)
			}
			if len(passwordBytes) != 24 {
				return fmt.Errorf("expected 24-char generated password, got %d", len(passwordBytes))
			}
			return nil
		}),
	}
}

func checkFIPSKeystoreSecretEventuallyAbsentStep(k *test.K8sClient, es esv1.Elasticsearch, name, stillPresentFmt string) test.Step {
	return test.Step{
		Name: name,
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			nn := types.NamespacedName{Namespace: es.Namespace, Name: esv1.KeystorePasswordSecret(es.Name)}
			err := k.Client.Get(context.Background(), nn, &secret)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf(stillPresentFmt, secret.Namespace, secret.Name)
		}),
	}
}

func checkFIPSKeystoreSecretAbsentStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return checkFIPSKeystoreSecretEventuallyAbsentStep(k, es,
		"FIPS keystore password secret should not be created",
		"unexpected secret %s/%s exists")
}

func checkFIPSKeystoreSecretDeletedStep(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return checkFIPSKeystoreSecretEventuallyAbsentStep(k, es,
		"FIPS keystore password secret should eventually be deleted",
		"secret %s/%s still exists")
}

func checkFIPSPodTemplateInjectedStep(k *test.K8sClient, es esv1.Elasticsearch, expectUserOverride bool) test.Step {
	stepName := "StatefulSet pod template should include FIPS keystore password Secret mount and env"
	if expectUserOverride {
		stepName = "StatefulSet pod template should omit operator-managed FIPS keystore password mount and env when user override is set"
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

			esContainer := pod.ContainerByName(sset.Spec.Template.Spec, esv1.ElasticsearchContainerName)
			if esContainer == nil {
				return fmt.Errorf("container %q not found", esv1.ElasticsearchContainerName)
			}

			if expectUserOverride {
				return checkFIPSPodTemplateUserOverride(&sset.Spec.Template.Spec, esContainer)
			}
			return checkFIPSPodTemplateOperatorManaged(&sset.Spec.Template.Spec, esContainer)
		}),
	}
}

func checkFIPSPodTemplateUserOverride(podSpec *corev1.PodSpec, esContainer *corev1.Container) error {
	hasFIPSVolume := slices.ContainsFunc(podSpec.Volumes, func(v corev1.Volume) bool {
		return v.Name == esvolume.KeystorePasswordSecretVolumeName
	})
	mainHasFIPSMount := slices.ContainsFunc(esContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == esvolume.KeystorePasswordSecretVolumeName && vm.MountPath == esvolume.KeystorePasswordSecretVolumeMountPath
	})

	initHasFIPSManagedArtifacts := false
	if keystoreInit := pod.InitContainerByName(*podSpec, commonkeystore.InitContainerName); keystoreInit != nil {
		initHasFIPSMount := slices.ContainsFunc(keystoreInit.VolumeMounts, func(vm corev1.VolumeMount) bool {
			return vm.Name == esvolume.KeystorePasswordSecretVolumeName && vm.MountPath == esvolume.KeystorePasswordSecretVolumeMountPath
		})
		initHasFIPSManagedArtifacts = initHasFIPSMount || keystoreInitCommandUsesPasswordFile(keystoreInit.Command)
	}

	mainHasPasswordFileEnv := slices.ContainsFunc(esContainer.Env, func(e corev1.EnvVar) bool { return e.Name == "KEYSTORE_PASSWORD_FILE" })

	checks := []struct {
		condition bool
		err       error
	}{
		{
			condition: !hasFIPSVolume && !mainHasFIPSMount,
			err:       fmt.Errorf("operator-managed FIPS keystore password volume or Elasticsearch container mount unexpectedly present in pod template"),
		},
		{
			condition: !initHasFIPSManagedArtifacts,
			err:       fmt.Errorf("operator-managed FIPS keystore password mount or password-file bootstrap unexpectedly present on keystore init container"),
		},
		{
			condition: mainHasPasswordFileEnv,
			err:       fmt.Errorf("expected user-provided KEYSTORE_PASSWORD_FILE env var to be present"),
		},
	}
	for _, check := range checks {
		if !check.condition {
			return check.err
		}
	}
	return nil
}

func checkFIPSPodTemplateOperatorManaged(podSpec *corev1.PodSpec, esContainer *corev1.Container) error {
	hasFIPSVolume := slices.ContainsFunc(podSpec.Volumes, func(v corev1.Volume) bool {
		return v.Name == esvolume.KeystorePasswordSecretVolumeName
	})
	mainHasFIPSMount := slices.ContainsFunc(esContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == esvolume.KeystorePasswordSecretVolumeName && vm.MountPath == esvolume.KeystorePasswordSecretVolumeMountPath
	})

	keystoreInitContainer := pod.InitContainerByName(*podSpec, commonkeystore.InitContainerName)
	if keystoreInitContainer == nil {
		return fmt.Errorf("container %q not found", commonkeystore.InitContainerName)
	}
	initHasFIPSMount := slices.ContainsFunc(keystoreInitContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == esvolume.KeystorePasswordSecretVolumeName && vm.MountPath == esvolume.KeystorePasswordSecretVolumeMountPath
	})
	mainHasPassphraseEnv := slices.ContainsFunc(esContainer.Env, func(e corev1.EnvVar) bool {
		return e.Name == "KEYSTORE_PASSWORD_FILE" &&
			e.Value == eskeystorepassword.PasswordFile
	})
	mainHasPasswordEnvFromSecret := slices.ContainsFunc(esContainer.Env, func(e corev1.EnvVar) bool {
		return e.Name == "KEYSTORE_PASSWORD" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil
	})
	script := strings.Join(keystoreInitContainer.Command, " ")

	checks := []struct {
		condition bool
		err       error
	}{
		{
			condition: hasFIPSVolume,
			err:       fmt.Errorf("missing %s volume", esvolume.KeystorePasswordSecretVolumeName),
		},
		{
			condition: mainHasFIPSMount,
			err:       fmt.Errorf("missing %s mount on Elasticsearch container", esvolume.KeystorePasswordSecretVolumeMountPath),
		},
		{
			condition: initHasFIPSMount,
			err:       fmt.Errorf("missing %s mount on init container", esvolume.KeystorePasswordSecretVolumeMountPath),
		},
		{
			condition: mainHasPassphraseEnv,
			err:       fmt.Errorf("missing KEYSTORE_PASSWORD_FILE=%s env var", eskeystorepassword.PasswordFile),
		},
		{
			condition: !mainHasPasswordEnvFromSecret,
			err:       fmt.Errorf("unexpected KEYSTORE_PASSWORD secret env var on Elasticsearch container"),
		},
		{
			condition: keystoreInitCommandUsesPasswordFile(keystoreInitContainer.Command),
			err:       fmt.Errorf("expected keystore init to use password-file bootstrap, got command %q", script),
		},
	}
	for _, check := range checks {
		if !check.condition {
			return check.err
		}
	}
	return nil
}

// keystoreInitCommandUsesPasswordFile reports whether the keystore init container command
// appears to run elasticsearch-keystore create -p with KEYSTORE_PASSWORD read from a file,
// matching the operator-managed FIPS bootstrap script shape.
func keystoreInitCommandUsesPasswordFile(cmd []string) bool {
	script := strings.Join(cmd, " ")
	return strings.Contains(script, "create -p") &&
		strings.Contains(script, `KEYSTORE_PASSWORD=$(cat "`)
}
