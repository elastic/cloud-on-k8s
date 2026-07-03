// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// S3 key names and values used across file-based secure settings tests.
const (
	s3AccessKey             = "s3.client.default.access_key"
	s3SecretKey             = "s3.client.default.secret_key"
	s3InitialAccessKeyValue = "AKIAIOSFODNN7EXAMPLE"
	s3InitialSecretKeyValue = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	s3UpdatedAccessKeyValue = "UPDATEDKEY987654321EXAMPLE"
	s3UpdatedSecretKeyValue = "updatedSecretKeyValue/EXAMPLE"
)

// TestFileBasedSecureSettings validates that on ES >= 9.5, when the opt-in annotation is
// set, spec.secureSettings are delivered via cluster_secrets in the file-based settings
// JSON rather than the keystore init container, enabling hot-reload without pod restarts.
func TestFileBasedSecureSettings(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(esversion.FileBasedSecureSettingsMinVersion) {
		t.Skipf("skipping: file-based secure settings require ES >= %s, got %s",
			esversion.FileBasedSecureSettingsMinVersion, test.Ctx().ElasticStackVersion)
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	secureSettingsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "file-based-secure-settings-src",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			s3AccessKey: []byte(s3InitialAccessKeyValue),
			s3SecretKey: []byte(s3InitialSecretKeyValue),
		},
	}

	b := elasticsearch.NewBuilder("test-es-file-secure").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettingsSecret.Name).
		WithAnnotation(esv1.FileBasedSecureSettingsAnnotation, "true")

	podUIDs := map[types.UID]struct{}{}
	var versionBeforeUpdate int64 = -1

	test.StepList{}.
		WithStep(createSourceSecretStep(ctx, k, &secureSettingsSecret)).
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithSteps(test.StepList{
			checkKeystoreInitContainerAbsent(ctx, k, b),
			checkInitialClusterSecretsStep(ctx, k, b),
			recordPodUIDsAndFileSettingsVersionStep(ctx, k, b, podUIDs, &versionBeforeUpdate),
			updateSecureSettingsSecretStep(ctx, k, &secureSettingsSecret),
			checkFileSettingsVersionAdvancedStep(ctx, k, b, &versionBeforeUpdate),
			checkUpdatedClusterSecretsStep(ctx, k, b),
			checkPodsNotRestartedStep(k, b, podUIDs),
			removeFileBasedAnnotationStep(ctx, k, b),
			checkKeystoreInitContainerPresentStep(ctx, k, b),
			checkSecureSettingsSecretExistsStep(ctx, k, b),
			checkClusterSecretsClearedStep(ctx, k, b),
			deleteSourceSecretStep(ctx, k, &secureSettingsSecret),
		}).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestFileBasedSecureSettingsFIPS is identical to TestFileBasedSecureSettings but runs with
// xpack.security.fips_mode.enabled: true. It validates that file-based secure settings and
// hot-reload work under FIPS, and that the operator does not create a keystore password secret
// (since the file-based path skips keystore management entirely).
func TestFileBasedSecureSettingsFIPS(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(esversion.FileBasedSecureSettingsMinVersion) {
		t.Skipf("skipping: file-based secure settings require ES >= %s, got %s",
			esversion.FileBasedSecureSettingsMinVersion, test.Ctx().ElasticStackVersion)
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	secureSettingsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "file-based-secure-settings-fips-src",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			s3AccessKey: []byte(s3InitialAccessKeyValue),
			s3SecretKey: []byte(s3InitialSecretKeyValue),
		},
	}

	b := elasticsearch.NewBuilder("test-es-fbs-fips").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes().
		WithAdditionalConfig(map[string]map[string]any{
			"masterdata": {
				fipsSetting: true,
			},
		}).
		WithESSecureSettings(secureSettingsSecret.Name).
		WithAnnotation(esv1.FileBasedSecureSettingsAnnotation, "true")

	podUIDs := map[types.UID]struct{}{}
	var versionBeforeUpdate int64 = -1

	test.StepList{}.
		WithStep(createSourceSecretStep(ctx, k, &secureSettingsSecret)).
		WithSteps(b.InitTestSteps(k)).
		WithStep(deleteFIPSKeystoreSecretStep(k, b.Elasticsearch)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithSteps(test.StepList{
			checkKeystoreInitContainerAbsent(ctx, k, b),
			checkFIPSKeystoreSecretAbsentStep(k, b.Elasticsearch),
			checkInitialClusterSecretsStep(ctx, k, b),
			recordPodUIDsAndFileSettingsVersionStep(ctx, k, b, podUIDs, &versionBeforeUpdate),
			updateSecureSettingsSecretStep(ctx, k, &secureSettingsSecret),
			checkFileSettingsVersionAdvancedStep(ctx, k, b, &versionBeforeUpdate),
			checkUpdatedClusterSecretsStep(ctx, k, b),
			checkPodsNotRestartedStep(k, b, podUIDs),
			removeFileBasedAnnotationStep(ctx, k, b),
			checkKeystoreInitContainerPresentStep(ctx, k, b),
			checkSecureSettingsSecretExistsStep(ctx, k, b),
			checkClusterSecretsClearedStep(ctx, k, b),
			checkFIPSKeystoreSecretCreatedStep(k, b.Elasticsearch),
			deleteSourceSecretStep(ctx, k, &secureSettingsSecret),
		}).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}

// fileSettingsJSON is the structure of the settings.json file mounted into ES pods.
type fileSettingsJSON struct {
	State struct {
		ClusterSecrets struct {
			StringSecrets map[string]string `json:"string_secrets"`
		} `json:"cluster_secrets"`
	} `json:"state"`
}

// checkClusterSecretsApplied verifies two things atomically:
//  1. The ECK-managed file-settings Secret contains cluster_secrets.string_secrets with
//     exactly the key-value pairs in want.
//  2. The ES _cluster/state API reports the file settings as applied (version >= 0, no errors).
//
// Combining both into a single step avoids a TOCTOU race where the K8s secret has been
// updated but ES has not yet reloaded, or vice versa.
func checkClusterSecretsApplied(ctx context.Context, k *test.K8sClient, es esv1.Elasticsearch, want map[string]string) error {
	// 1. Check the K8s secret content.
	var secret corev1.Secret
	if err := k.Client.Get(ctx,
		types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)},
		&secret,
	); err != nil {
		return err
	}
	raw, ok := secret.Data["settings.json"]
	if !ok {
		return fmt.Errorf("settings.json not found in secret %s", esv1.FileSettingsSecretName(es.Name))
	}
	var parsed fileSettingsJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parsing settings.json: %w", err)
	}
	got := parsed.State.ClusterSecrets.StringSecrets
	if len(got) != len(want) {
		return fmt.Errorf("cluster_secrets.string_secrets: expected %d keys, got %d", len(want), len(got))
	}
	for key, wantVal := range want {
		gotVal, exists := got[key]
		if !exists {
			return fmt.Errorf("key %q not found in cluster_secrets.string_secrets", key)
		}
		if gotVal != wantVal {
			return fmt.Errorf("key %q: expected %q, got %q", key, wantVal, gotVal)
		}
	}

	// 2. Check ES has applied the file settings.
	esClient, err := elasticsearch.NewElasticsearchClient(es, k)
	if err != nil {
		return err
	}
	defer esClient.Close()
	state, err := esClient.GetClusterState(ctx)
	if err != nil {
		return err
	}
	fs := state.Metadata.ReservedState.FileSettings
	if fs.Version < 0 {
		return fmt.Errorf("file settings not yet applied: version=%d", fs.Version)
	}
	if fs.Errors != nil {
		return fmt.Errorf("file settings applied with errors: %+v", fs.Errors)
	}
	return nil
}

func createSourceSecretStep(ctx context.Context, k *test.K8sClient, secret *corev1.Secret) test.Step {
	return test.Step{
		Name: "Create source secure settings secret",
		Test: test.Eventually(func() error {
			return k.CreateOrUpdateSecrets(*secret)
		}),
	}
}

func checkKeystoreInitContainerAbsent(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "Pod template must not contain the keystore init container",
		Test: test.Eventually(func() error {
			var sset appsv1.StatefulSet
			if err := k.Client.Get(ctx,
				types.NamespacedName{
					Namespace: b.Elasticsearch.Namespace,
					Name:      esv1.StatefulSet(b.Elasticsearch.Name, b.Elasticsearch.Spec.NodeSets[0].Name),
				},
				&sset,
			); err != nil {
				return err
			}
			if c := pod.InitContainerByName(sset.Spec.Template.Spec, keystore.InitContainerName); c != nil {
				return fmt.Errorf("keystore init container %q must not be present for ES >= 9.5 with file-based secure settings", keystore.InitContainerName)
			}
			return nil
		}),
	}
}

func checkInitialClusterSecretsStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "File-settings secret must contain cluster_secrets with both S3 keys and ES must have applied them without errors",
		Test: test.Eventually(func() error {
			return checkClusterSecretsApplied(ctx, k, b.Elasticsearch, map[string]string{
				s3AccessKey: s3InitialAccessKeyValue,
				s3SecretKey: s3InitialSecretKeyValue,
			})
		}),
	}
}

func recordPodUIDsAndFileSettingsVersionStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder, podUIDs map[types.UID]struct{}, versionOut *int64) test.Step {
	return test.Step{
		Name: "Record pod UIDs and file settings version before secret update",
		Test: test.Eventually(func() error {
			esClient, err := elasticsearch.NewElasticsearchClient(b.Elasticsearch, k)
			if err != nil {
				return err
			}
			defer esClient.Close()
			state, err := esClient.GetClusterState(ctx)
			if err != nil {
				return err
			}
			*versionOut = state.Metadata.ReservedState.FileSettings.Version

			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			if err != nil {
				return err
			}
			for _, p := range pods {
				podUIDs[p.UID] = struct{}{}
			}
			if len(podUIDs) == 0 {
				return fmt.Errorf("no pods found")
			}
			return nil
		}),
	}
}

func updateSecureSettingsSecretStep(ctx context.Context, k *test.K8sClient, secret *corev1.Secret) test.Step {
	return test.Step{
		Name: "Update S3 credentials in source secret",
		Test: test.Eventually(func() error {
			secret.Data = map[string][]byte{
				s3AccessKey: []byte(s3UpdatedAccessKeyValue),
				s3SecretKey: []byte(s3UpdatedSecretKeyValue),
			}
			return k.Client.Update(ctx, secret)
		}),
	}
}

func checkFileSettingsVersionAdvancedStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder, versionBefore *int64) test.Step {
	return test.Step{
		Name: "ES file settings version must have advanced after secret update",
		Test: test.Eventually(func() error {
			esClient, err := elasticsearch.NewElasticsearchClient(b.Elasticsearch, k)
			if err != nil {
				return err
			}
			defer esClient.Close()
			state, err := esClient.GetClusterState(ctx)
			if err != nil {
				return err
			}
			v := state.Metadata.ReservedState.FileSettings.Version
			if v <= *versionBefore {
				return fmt.Errorf("file settings version not yet advanced: got %d, want > %d", v, *versionBefore)
			}
			return nil
		}),
	}
}

func checkUpdatedClusterSecretsStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "File-settings secret must reflect updated S3 credentials and ES must have reloaded them without errors",
		Test: test.Eventually(func() error {
			return checkClusterSecretsApplied(ctx, k, b.Elasticsearch, map[string]string{
				s3AccessKey: s3UpdatedAccessKeyValue,
				s3SecretKey: s3UpdatedSecretKeyValue,
			})
		}),
	}
}

func checkPodsNotRestartedStep(k *test.K8sClient, b elasticsearch.Builder, podUIDs map[types.UID]struct{}) test.Step {
	return test.Step{
		Name: "Pods must not have been restarted after secret update",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			if err != nil {
				return err
			}
			for _, p := range pods {
				if _, existed := podUIDs[p.UID]; !existed {
					return fmt.Errorf("pod %s has a new UID — it was restarted, expected hot-reload", p.Name)
				}
			}
			return nil
		}),
	}
}

func deleteSourceSecretStep(ctx context.Context, k *test.K8sClient, secret *corev1.Secret) test.Step {
	return test.Step{
		Name: "Delete source secure settings secret",
		Test: test.Eventually(func() error {
			err := k.Client.Delete(ctx, secret)
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}),
	}
}

func removeFileBasedAnnotationStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "Remove file-based secure settings annotation to flip back to keystore path",
		Test: test.Eventually(func() error {
			var es esv1.Elasticsearch
			if err := k.Client.Get(ctx, types.NamespacedName{Namespace: b.Elasticsearch.Namespace, Name: b.Elasticsearch.Name}, &es); err != nil {
				return err
			}
			delete(es.Annotations, esv1.FileBasedSecureSettingsAnnotation)
			return k.Client.Update(ctx, &es)
		}),
	}
}

func checkKeystoreInitContainerPresentStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "Pod template must contain the keystore init container after annotation removal",
		Test: test.Eventually(func() error {
			var sset appsv1.StatefulSet
			if err := k.Client.Get(ctx,
				types.NamespacedName{
					Namespace: b.Elasticsearch.Namespace,
					Name:      esv1.StatefulSet(b.Elasticsearch.Name, b.Elasticsearch.Spec.NodeSets[0].Name),
				},
				&sset,
			); err != nil {
				return err
			}
			if c := pod.InitContainerByName(sset.Spec.Template.Spec, keystore.InitContainerName); c == nil {
				return fmt.Errorf("keystore init container %q must be present after annotation removal", keystore.InitContainerName)
			}
			return nil
		}),
	}
}

func checkSecureSettingsSecretExistsStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "Operator-managed secure-settings Secret must be recreated after annotation removal",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			return k.Client.Get(ctx,
				types.NamespacedName{
					Namespace: b.Elasticsearch.Namespace,
					Name:      esv1.ESNamer.Suffix(b.Elasticsearch.Name, "secure-settings"),
				},
				&secret,
			)
		}),
	}
}

func checkClusterSecretsClearedStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "cluster_secrets must be cleared from file-settings Secret and ES must have applied the updated settings without errors",
		Test: test.Eventually(func() error {
			// 1. Check the K8s Secret has empty cluster_secrets.
			var secret corev1.Secret
			if err := k.Client.Get(ctx,
				types.NamespacedName{Namespace: b.Elasticsearch.Namespace, Name: esv1.FileSettingsSecretName(b.Elasticsearch.Name)},
				&secret,
			); err != nil {
				return err
			}
			raw, ok := secret.Data["settings.json"]
			if !ok {
				return nil
			}
			var parsed fileSettingsJSON
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return fmt.Errorf("parsing settings.json: %w", err)
			}
			if len(parsed.State.ClusterSecrets.StringSecrets) > 0 {
				return fmt.Errorf("cluster_secrets.string_secrets still contains %d keys after annotation removal", len(parsed.State.ClusterSecrets.StringSecrets))
			}
			// 2. Verify via ES client that the current file settings version was applied without errors.
			// This is faster than polling the CRD status and confirms ES is up and has processed the cleared settings.
			esClient, err := elasticsearch.NewElasticsearchClient(b.Elasticsearch, k)
			if err != nil {
				return err
			}
			defer esClient.Close()
			state, err := esClient.GetClusterState(ctx)
			if err != nil {
				return err
			}
			fs := state.Metadata.ReservedState.FileSettings
			if fs.Version < 0 {
				return fmt.Errorf("file settings not yet applied: version=%d", fs.Version)
			}
			if fs.Errors != nil {
				return fmt.Errorf("file settings applied with errors: %+v", fs.Errors)
			}
			return nil
		}),
	}
}
