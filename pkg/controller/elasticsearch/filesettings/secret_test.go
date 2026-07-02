// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	commonlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_FileSettingsSecret_ApplyPolicy(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: esNsn.Namespace,
		Name:      esNsn.Name,
	}}
	policy := policyv1alpha1.ElasticsearchConfigPolicySpec{
		ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
	}

	fakeClient := k8s.NewFakeClient()

	// No policy: empty settings
	fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)

	var secret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	require.NoError(t, err)
	assert.Equal(t, 0, len(parseSettings(t, secret).State.ClusterSettings.Data))

	// With policy
	fs2, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs2.ApplyPolicy(policy, nil)
	require.NoError(t, err)
	err = fs2.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	require.NoError(t, err)
	assert.Equal(t, 1, len(parseSettings(t, secret).State.ClusterSettings.Data))
}

func Test_FileSettingsSecret_VersionUnchanged(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: esNsn.Namespace,
		Name:      esNsn.Name,
	}}

	fakeClient := k8s.NewFakeClient()

	// Create empty settings
	fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)
	v1 := fs.Version()

	// Load again, save with no changes: version should stay the same
	fs2, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs2.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)
	assert.Equal(t, v1, fs2.Version())

	// Load again, apply policy: version should change
	fs3, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs3.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{
		ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
	}, nil)
	require.NoError(t, err)
	err = fs3.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)
	assert.NotEqual(t, v1, fs3.Version())
}

func Test_FileSettingsSecret_ApplyPolicy_PreservesClusterSecrets(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}

	// Create a current secret that has cluster_secrets (written by ES controller)
	currentSettings := NewEmptySettings(1)
	currentSettings.State.ClusterSecrets = &commonv1.Config{Data: map[string]any{
		"string_secrets": map[string]any{"s3": map[string]any{"key": "value"}},
	}}
	settingsBytes, err := json.Marshal(currentSettings)
	require.NoError(t, err)

	currentSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   esNsn.Namespace,
			Name:        "esName-es-file-settings",
			Annotations: map[string]string{commonannotation.SettingsHashAnnotationName: currentSettings.hash()},
		},
		Data: map[string][]byte{SettingsSecretKey: settingsBytes},
	}

	fakeClient := k8s.NewFakeClient(currentSecret)

	// ApplyPolicy must not wipe cluster_secrets, which is owned by the ES controller.
	fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{}, nil)
	require.NoError(t, err)

	assert.NotNil(t, fs.settings.State.ClusterSecrets, "cluster_secrets must be preserved by ApplyPolicy")
	assert.Equal(t, currentSettings.State.ClusterSecrets, fs.settings.State.ClusterSecrets)
}

func Test_Reset_ClearsSCPManagedFields(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}

	// Create a current secret with cluster_secrets and cluster_settings
	currentSettings := NewEmptySettings(1)
	currentSettings.State.ClusterSecrets = &commonv1.Config{Data: map[string]any{
		"string_secrets": map[string]any{"s3": map[string]any{"key": "value"}},
	}}
	currentSettings.State.ClusterSettings = &commonv1.Config{Data: map[string]any{
		"indices.recovery.max_bytes_per_sec": "100mb",
	}}
	settingsBytes, err := json.Marshal(currentSettings)
	require.NoError(t, err)

	currentSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   esNsn.Namespace,
			Name:        "esName-es-file-settings",
			Annotations: map[string]string{commonannotation.SettingsHashAnnotationName: currentSettings.hash()},
		},
		Data: map[string][]byte{SettingsSecretKey: settingsBytes},
	}

	fakeClient := k8s.NewFakeClient(currentSecret)

	// Reset clears all SCP-managed fields (including cluster_secrets which came from prior SCP state)
	fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	assert.NotNil(t, fs.settings.State.ClusterSecrets, "cluster_secrets should be loaded from current")

	fs.Reset()
	// cluster_secrets is owned by the ES controller, not SCP — Reset must not clear it.
	assert.NotNil(t, fs.settings.State.ClusterSecrets, "Reset must not clear cluster_secrets (ES controller owns it)")
	assert.Empty(t, fs.settings.State.ClusterSettings.Data, "Reset should clear SCP-managed cluster_settings")
}

func Test_ApplyEmptyPolicy_ClearsSCPManagedFields(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: esNsn.Namespace,
		Name:      esNsn.Name,
		UID:       "test-uid",
	}}

	// Create a current secret with cluster_settings (SCP-managed)
	currentSettings := NewEmptySettings(1)
	currentSettings.State.ClusterSettings = &commonv1.Config{Data: map[string]any{
		"indices.recovery.max_bytes_per_sec": "100mb",
	}}
	settingsBytes, err := json.Marshal(currentSettings)
	require.NoError(t, err)

	currentSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   esNsn.Namespace,
			Name:        esv1.FileSettingsSecretName(esNsn.Name),
			Annotations: map[string]string{commonannotation.SettingsHashAnnotationName: currentSettings.hash()},
		},
		Data: map[string][]byte{SettingsSecretKey: settingsBytes},
	}

	fakeClient := k8s.NewFakeClient(&es, currentSecret)

	// Simulate last SCP owner removed: ApplyPolicy with empty policy should
	// clear all SCP-managed fields.
	fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{}, nil)
	require.NoError(t, err)
	err = fs.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)

	// Verify the saved Secret
	var secret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Namespace: esNsn.Namespace,
		Name:      esv1.FileSettingsSecretName(esNsn.Name),
	}, &secret)
	require.NoError(t, err)

	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)

	// SCP-managed fields cleared
	assert.Empty(t, settings.State.ClusterSettings.Data, "cluster_settings should be empty after clearing policy")
}

func Test_SecureSettings_RoundTrip(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: esNsn.Namespace,
		Name:      esNsn.Name,
	}}

	fakeClient := k8s.NewFakeClient()

	// No secure settings
	fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)

	var secret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	require.NoError(t, err)
	secureSettings, err := getSecureSettings(secret)
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	// With secure settings via ApplyPolicy
	fs2, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
	require.NoError(t, err)
	err = fs2.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{}, []commonv1.NamespacedSecretSource{
		{Namespace: "otherNs", SecretName: "secure-settings-secret"},
	})
	require.NoError(t, err)
	err = fs2.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	require.NoError(t, err)
	secureSettings, err = getSecureSettings(secret)
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{{Namespace: "otherNs", SecretName: "secure-settings-secret"}}, secureSettings)
}

func Test_Save_SecureSettingsAnnotation(t *testing.T) {
	tests := []struct {
		name                    string
		useAdditiveMetadata     bool
		wantAnnotationPreserved bool
	}{
		{
			name:                    "WithAdditiveMetadata preserves annotation (ES controller)",
			useAdditiveMetadata:     true,
			wantAnnotationPreserved: true,
		},
		{
			name:                    "without WithAdditiveMetadata removes annotation (SCP controller)",
			useAdditiveMetadata:     false,
			wantAnnotationPreserved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
			es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Namespace: esNsn.Namespace,
				Name:      esNsn.Name,
				UID:       "test-uid",
			}}

			existingSettings := NewEmptySettings(1)
			settingsBytes, err := json.Marshal(existingSettings)
			require.NoError(t, err)

			secureSettingsSources := []commonv1.NamespacedSecretSource{
				{Namespace: "esNs", SecretName: "my-secure-settings"},
			}
			secureSettingsJSON, err := json.Marshal(secureSettingsSources)
			require.NoError(t, err)

			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: esNsn.Namespace,
					Name:      esv1.FileSettingsSecretName(esNsn.Name),
					Annotations: map[string]string{
						commonannotation.SettingsHashAnnotationName:          existingSettings.hash(),
						commonannotation.SecureSettingsSecretsAnnotationName: string(secureSettingsJSON),
					},
				},
				Data: map[string][]byte{SettingsSecretKey: settingsBytes},
			}

			fakeClient := k8s.NewFakeClient(&es, existingSecret)

			fs, err := Load(context.Background(), fakeClient, esNsn, metadata.Metadata{})
			require.NoError(t, err)

			if tt.useAdditiveMetadata {
				err = fs.Save(context.Background(), fakeClient, &es, WithAdditiveMetadata())
			} else {
				err = fs.Save(context.Background(), fakeClient, &es)
			}
			require.NoError(t, err)

			var secret corev1.Secret
			err = fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: esNsn.Namespace,
				Name:      esv1.FileSettingsSecretName(esNsn.Name),
			}, &secret)
			require.NoError(t, err)

			secureSettings, err := getSecureSettings(secret)
			require.NoError(t, err)

			if tt.wantAnnotationPreserved {
				assert.Equal(t, secureSettingsSources, secureSettings,
					"secure-settings-secrets annotation should be preserved")
			} else {
				assert.Empty(t, secureSettings,
					"secure-settings-secrets annotation should be removed")
			}
		})
	}
}

func parseSettings(t *testing.T, secret corev1.Secret) Settings {
	t.Helper()
	var settings Settings
	err := json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	assert.NoError(t, err)
	return settings
}

func Test_buildSecret(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
	meta := metadata.Metadata{
		Labels:      map[string]string{"propagated-label": "v1"},
		Annotations: map[string]string{"propagated-annotation": "v2"},
	}

	fs := &Secret{
		es:       esNsn,
		meta:     meta,
		settings: NewEmptySettings(42),
		version:  42,
	}

	secret, err := fs.buildSecret("some-hash")
	require.NoError(t, err)

	assert.Equal(t, esNsn.Namespace, secret.Namespace)
	assert.Equal(t, esv1.FileSettingsSecretName(esNsn.Name), secret.Name)

	// Settings payload is present.
	assert.NotEmpty(t, secret.Data[SettingsSecretKey])

	// Propagated metadata is merged in.
	assert.Equal(t, "v1", secret.Labels["propagated-label"])
	assert.Equal(t, "v2", secret.Annotations["propagated-annotation"])

	// Hash annotation is set from the argument.
	assert.Equal(t, "some-hash", secret.Annotations[commonannotation.SettingsHashAnnotationName])

	// Operator-managed labels are applied.
	assert.Equal(t, commonlabel.OrphanSecretResetOnPolicyDelete, secret.Labels[commonlabel.StackConfigPolicyOnDeleteLabelName])
	assert.Equal(t, commonv1.RestrictWatchedResourcesLabelValue, secret.Labels[commonv1.RestrictWatchedResourcesLabelName])
}
