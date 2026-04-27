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
	fs, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)

	var secret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	require.NoError(t, err)
	assert.Equal(t, 0, len(parseSettings(t, secret).State.ClusterSettings.Data))

	// With policy
	fs2, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
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
	fs, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)
	v1 := fs.Version()

	// Load again, save with no changes: version should stay the same
	fs2, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
	require.NoError(t, err)
	err = fs2.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)
	assert.Equal(t, v1, fs2.Version())

	// Load again, apply policy: version should change
	fs3, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
	require.NoError(t, err)
	err = fs3.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{
		ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
	}, nil)
	require.NoError(t, err)
	err = fs3.Save(context.Background(), fakeClient, &es)
	require.NoError(t, err)
	assert.NotEqual(t, v1, fs3.Version())
}

func Test_FileSettingsSecret_PreservesClusterSecrets(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}

	// Create a current secret that has cluster_secrets (written by ES controller)
	currentSettings := NewEmptySettings(1, true)
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

	// SCP rebuilds with ApplyPolicy: cluster_secrets should be preserved for stateless
	fs, err := Load(context.Background(), fakeClient, esNsn, true, metadata.Metadata{})
	require.NoError(t, err)
	err = fs.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{}, nil)
	require.NoError(t, err)

	// Verify settings before save
	assert.NotNil(t, fs.settings.State.ClusterSecrets, "cluster_secrets should be preserved for stateless")
	assert.Equal(t, currentSettings.State.ClusterSecrets.Data, fs.settings.State.ClusterSecrets.Data)

	// Stateful: cluster_secrets should NOT be preserved
	fs2, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
	require.NoError(t, err)
	err = fs2.ApplyPolicy(policyv1alpha1.ElasticsearchConfigPolicySpec{}, nil)
	require.NoError(t, err)
	assert.Nil(t, fs2.settings.State.ClusterSecrets, "cluster_secrets should not be preserved for stateful")
}

func Test_Reset_PreservesClusterSecretsForStateless(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}

	// Create a current secret with cluster_secrets and cluster_settings
	currentSettings := NewEmptySettings(1, true)
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

	// Reset preserves cluster_secrets for stateless (managed by ES controller, not SCP)
	fs, err := Load(context.Background(), fakeClient, esNsn, true, metadata.Metadata{})
	require.NoError(t, err)
	assert.NotNil(t, fs.settings.State.ClusterSecrets, "cluster_secrets should be loaded from current")

	fs.Reset()
	assert.NotNil(t, fs.settings.State.ClusterSecrets, "Reset should preserve cluster_secrets for stateless")
	assert.Empty(t, fs.settings.State.ClusterSettings.Data, "Reset should clear SCP-managed cluster_settings")

	// Reset drops cluster_secrets for stateful (not applicable)
	fs2, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
	require.NoError(t, err)
	fs2.Reset()
	assert.Nil(t, fs2.settings.State.ClusterSecrets, "Reset should not preserve cluster_secrets for stateful")
}

func Test_ApplyEmptyPolicy_PreservesClusterSecretsForStateless(t *testing.T) {
	esNsn := types.NamespacedName{Namespace: "esNs", Name: "esName"}
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: esNsn.Namespace,
		Name:      esNsn.Name,
		UID:       "test-uid",
	}}

	// Create a current secret with cluster_secrets and cluster_settings (SCP-managed)
	currentSettings := NewEmptySettings(1, true)
	currentSettings.State.ClusterSecrets = &commonv1.Config{Data: map[string]any{
		"string_secrets": map[string]any{"gcs": map[string]any{"credentials": "secret"}},
	}}
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
	// clear SCP-managed fields but preserve cluster_secrets for stateless.
	fs, err := Load(context.Background(), fakeClient, esNsn, true, metadata.Metadata{})
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

	// cluster_secrets preserved
	require.NotNil(t, settings.State.ClusterSecrets, "cluster_secrets should be preserved")
	stringSecrets, ok := settings.State.ClusterSecrets.Data["string_secrets"].(map[string]any)
	require.True(t, ok)
	gcs, ok := stringSecrets["gcs"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "secret", gcs["credentials"])

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
	fs, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
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
	fs2, err := Load(context.Background(), fakeClient, esNsn, false, metadata.Metadata{})
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

func parseSettings(t *testing.T, secret corev1.Secret) Settings {
	t.Helper()
	var settings Settings
	err := json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	assert.NoError(t, err)
	return settings
}
