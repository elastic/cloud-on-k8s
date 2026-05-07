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
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func testES() esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es",
			Namespace: "ns",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.4.0",
			Mode:    esv1.ElasticsearchModeStateless,
		},
	}
}

func testClusterSecrets() *commonv1.Config {
	return &commonv1.Config{Data: map[string]any{
		"string_secrets": map[string]any{
			"s3": map[string]any{
				"client": map[string]any{
					"default": map[string]any{
						"access_key": "AKIA...",
						"secret_key": "secret...",
					},
				},
			},
		},
	}}
}

func TestReconcileClusterSecrets_CreateWhenNotFound(t *testing.T) {
	es := testES()
	clusterSecrets := testClusterSecrets()

	client := k8s.NewFakeClient(&es)
	err := ReconcileClusterSecrets(context.Background(), client, es, clusterSecrets)
	require.NoError(t, err)

	// Verify the secret was created
	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	// Verify cluster_secrets is set
	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	assert.Equal(t, clusterSecrets.Data, settings.State.ClusterSecrets.Data)
}

func TestReconcileClusterSecrets_NoOpWhenUnchanged(t *testing.T) {
	es := testES()
	clusterSecrets := testClusterSecrets()

	client := k8s.NewFakeClient(&es)

	// First reconciliation creates the secret
	err := ReconcileClusterSecrets(context.Background(), client, es, clusterSecrets)
	require.NoError(t, err)

	// Get the secret and note its version
	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	originalVersion := settings.Metadata.Version

	// Second reconciliation with same data should be a no-op
	err = ReconcileClusterSecrets(context.Background(), client, es, clusterSecrets)
	require.NoError(t, err)

	// Version should not have changed
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	assert.Equal(t, originalVersion, settings.Metadata.Version)
}

func TestReconcileClusterSecrets_UpdateOnChange(t *testing.T) {
	es := testES()
	clusterSecrets := testClusterSecrets()

	client := k8s.NewFakeClient(&es)

	// First reconciliation
	err := ReconcileClusterSecrets(context.Background(), client, es, clusterSecrets)
	require.NoError(t, err)

	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	originalVersion := settings.Metadata.Version

	// Second reconciliation with different data
	newSecrets := &commonv1.Config{Data: map[string]any{
		"string_secrets": map[string]any{
			"gcs": map[string]any{"credentials": "new-creds"},
		},
	}}
	err = ReconcileClusterSecrets(context.Background(), client, es, newSecrets)
	require.NoError(t, err)

	// Version should have been bumped
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	assert.NotEqual(t, originalVersion, settings.Metadata.Version)
	assert.Equal(t, newSecrets.Data, settings.State.ClusterSecrets.Data)
}

func TestReconcileClusterSecrets_PreservesOtherFields(t *testing.T) {
	es := testES()

	// Create a secret that already has cluster_settings (set by SCP)
	existingSettings := NewEmptySettings(1, true)
	existingSettings.State.ClusterSettings = &commonv1.Config{Data: map[string]any{"indices.recovery.max_bytes_per_sec": "100mb"}}
	settingsBytes, err := json.Marshal(existingSettings)
	require.NoError(t, err)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.FileSettingsSecretName(es.Name),
			Namespace: es.Namespace,
			Annotations: map[string]string{
				commonannotation.SettingsHashAnnotationName: existingSettings.hash(),
			},
		},
		Data: map[string][]byte{
			SettingsSecretKey: settingsBytes,
		},
	}

	client := k8s.NewFakeClient(&es, existingSecret)

	// Reconcile cluster_secrets
	clusterSecrets := testClusterSecrets()
	err = ReconcileClusterSecrets(context.Background(), client, es, clusterSecrets)
	require.NoError(t, err)

	// Verify cluster_settings is preserved
	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	assert.NotNil(t, settings.State.ClusterSettings)
	assert.Equal(t, "100mb", settings.State.ClusterSettings.Data["indices.recovery.max_bytes_per_sec"])
	assert.NotNil(t, settings.State.ClusterSecrets)
}

func TestReconcileClusterSecrets_PreservesSCPManagedMetadata(t *testing.T) {
	es := testES()

	// Create a secret with SCP-managed annotations and labels.
	existingSettings := NewEmptySettings(1, true)
	settingsBytes, err := json.Marshal(existingSettings)
	require.NoError(t, err)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.FileSettingsSecretName(es.Name),
			Namespace: es.Namespace,
			Labels: map[string]string{
				reconciler.SoftOwnerKindLabel: "StackConfigPolicy",
			},
			Annotations: map[string]string{
				commonannotation.SettingsHashAnnotationName:          existingSettings.hash(),
				reconciler.SoftOwnerRefsAnnotation:                   `["ns/test-policy"]`,
				commonannotation.SecureSettingsSecretsAnnotationName: "ns/my-secure-settings",
			},
		},
		Data: map[string][]byte{
			SettingsSecretKey: settingsBytes,
		},
	}

	client := k8s.NewFakeClient(&es, existingSecret)

	// Reconcile cluster_secrets
	clusterSecrets := testClusterSecrets()
	err = ReconcileClusterSecrets(context.Background(), client, es, clusterSecrets)
	require.NoError(t, err)

	// Verify SCP-managed annotations and labels are preserved.
	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	assert.Equal(t, `["ns/test-policy"]`, secret.Annotations[reconciler.SoftOwnerRefsAnnotation])
	assert.Equal(t, "ns/my-secure-settings", secret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName])
	assert.Equal(t, "StackConfigPolicy", secret.Labels[reconciler.SoftOwnerKindLabel])

	// Verify cluster_secrets was still updated.
	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	assert.Equal(t, clusterSecrets.Data, settings.State.ClusterSecrets.Data)
}

func TestReconcileClusterSecrets_ClearsWhenNil(t *testing.T) {
	es := testES()

	// Create a secret with existing cluster_secrets
	existingSettings := NewEmptySettings(1, true)
	existingSettings.State.ClusterSecrets = testClusterSecrets()
	settingsBytes, err := json.Marshal(existingSettings)
	require.NoError(t, err)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.FileSettingsSecretName(es.Name),
			Namespace: es.Namespace,
			Annotations: map[string]string{
				commonannotation.SettingsHashAnnotationName: existingSettings.hash(),
			},
		},
		Data: map[string][]byte{
			SettingsSecretKey: settingsBytes,
		},
	}

	client := k8s.NewFakeClient(&es, existingSecret)

	// Reconcile with nil cluster_secrets
	err = ReconcileClusterSecrets(context.Background(), client, es, nil)
	require.NoError(t, err)

	// Verify cluster_secrets is cleared
	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)

	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	require.NoError(t, err)
	assert.Nil(t, settings.State.ClusterSecrets)
}

func TestReconcileClusterSecrets_FailsOnMalformedSettings(t *testing.T) {
	es := testES()

	// Create a secret with malformed settings.json
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.FileSettingsSecretName(es.Name),
			Namespace: es.Namespace,
			Annotations: map[string]string{
				commonannotation.SettingsHashAnnotationName: "some-hash",
			},
		},
		Data: map[string][]byte{
			SettingsSecretKey: []byte(`{invalid json`),
		},
	}

	client := k8s.NewFakeClient(&es, existingSecret)

	// ReconcileClusterSecrets should fail instead of overwriting with empty settings
	err := ReconcileClusterSecrets(context.Background(), client, es, testClusterSecrets())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed")

	// Verify the secret was NOT modified
	var secret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}, &secret)
	require.NoError(t, err)
	assert.Equal(t, []byte(`{invalid json`), secret.Data[SettingsSecretKey])
}

func TestReconcileClusterSecrets_FailsOnMissingSettingsKey(t *testing.T) {
	es := testES()

	// Create a secret with no settings.json key
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.FileSettingsSecretName(es.Name),
			Namespace: es.Namespace,
		},
		Data: map[string][]byte{},
	}

	client := k8s.NewFakeClient(&es, existingSecret)

	// ReconcileClusterSecrets should fail instead of overwriting with empty settings
	err := ReconcileClusterSecrets(context.Background(), client, es, testClusterSecrets())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed")
}
