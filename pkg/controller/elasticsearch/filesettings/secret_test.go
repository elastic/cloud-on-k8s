// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func Test_NewSettingsSecret(t *testing.T) {
	es := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}
	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "policyNs",
			Name:      "policyName",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
			},
		},
	}

	// no policy
	expectedVersion := int64(1)
	secret, reconciledVersion, err := newSettingsSecret(expectedVersion, false, es, nil, nil, nil, metadata.Metadata{})
	assert.NoError(t, err)
	assert.Equal(t, "esNs", secret.Namespace)
	assert.Equal(t, "esName-es-file-settings", secret.Name)
	assert.Equal(t, 0, len(parseSettings(t, secret).State.ClusterSettings.Data))
	assert.Equal(t, expectedVersion, reconciledVersion)

	// policy
	expectedVersion = int64(2)
	secret, reconciledVersion, err = newSettingsSecret(expectedVersion, false, es, &secret, &policy.Spec.Elasticsearch, policy.GetElasticsearchNamespacedSecureSettings(), metadata.Metadata{})
	assert.NoError(t, err)
	assert.Equal(t, "esNs", secret.Namespace)
	assert.Equal(t, "esName-es-file-settings", secret.Name)
	assert.Equal(t, 1, len(parseSettings(t, secret).State.ClusterSettings.Data))
	assert.Equal(t, expectedVersion, reconciledVersion)
}

func Test_SettingsSecret_hasChanged(t *testing.T) {
	es := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}
	policy := policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{
		Namespace: "policyNs",
		Name:      "policyName",
	}}
	otherPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "otherPolicyNs",
			Name:      "otherPolicyName",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
			},
		}}

	expectedVersion := int64(1)
	expectedEmptySettings := NewEmptySettings(expectedVersion, false)

	// no policy -> emptySettings
	secret, reconciledVersion, err := newSettingsSecret(expectedVersion, false, es, nil, nil, nil, metadata.Metadata{})
	assert.NoError(t, err)
	assert.Equal(t, false, hasChanged(secret, expectedEmptySettings))
	assert.Equal(t, expectedVersion, reconciledVersion)

	// policy without settings -> emptySettings
	sameSettings := NewEmptySettings(expectedVersion, false)
	err = sameSettings.updateState(es, policy.Spec.Elasticsearch)
	assert.NoError(t, err)
	assert.Equal(t, false, hasChanged(secret, sameSettings))
	assert.Equal(t, strconv.FormatInt(expectedVersion, 10), sameSettings.Metadata.Version)

	// new policy -> settings changed
	newVersion := int64(2)
	newSettings := NewEmptySettings(newVersion, false)

	err = newSettings.updateState(es, otherPolicy.Spec.Elasticsearch)
	assert.NoError(t, err)
	assert.Equal(t, true, hasChanged(secret, newSettings))
	assert.Equal(t, strconv.FormatInt(newVersion, 10), newSettings.Metadata.Version)
}

func Test_newSettingsSecret_stateless_preserves_cluster_secrets(t *testing.T) {
	es := types.NamespacedName{Namespace: "esNs", Name: "esName"}

	// Create a current secret that has cluster_secrets (written by ES controller)
	currentSettings := NewEmptySettings(1, true)
	currentSettings.State.ClusterSecrets = &commonv1.Config{Data: map[string]any{
		"string_secrets": map[string]any{"s3": map[string]any{"key": "value"}},
	}}
	settingsBytes, err := json.Marshal(currentSettings)
	require.NoError(t, err)

	currentSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   es.Namespace,
			Name:        "esName-es-file-settings",
			Annotations: map[string]string{commonannotation.SettingsHashAnnotationName: currentSettings.hash()},
		},
		Data: map[string][]byte{SettingsSecretKey: settingsBytes},
	}

	// SCP rebuilds the secret with isStateless=true: cluster_secrets should be preserved
	secret, _, err := newSettingsSecret(2, true, es, currentSecret, nil, nil, metadata.Metadata{})
	require.NoError(t, err)

	rebuilt := parseSettings(t, secret)
	assert.NotNil(t, rebuilt.State.ClusterSecrets, "cluster_secrets should be preserved for stateless")
	assert.Equal(t, currentSettings.State.ClusterSecrets.Data, rebuilt.State.ClusterSecrets.Data)

	// Stateful: cluster_secrets should NOT be preserved
	secret, _, err = newSettingsSecret(3, false, es, currentSecret, nil, nil, metadata.Metadata{})
	require.NoError(t, err)

	rebuilt = parseSettings(t, secret)
	assert.Nil(t, rebuilt.State.ClusterSecrets, "cluster_secrets should not be preserved for stateful")
}

func Test_SettingsSecret_setSecureSettings_getSecureSettings(t *testing.T) {
	es := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}
	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "policyNs",
			Name:      "policyName",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SecureSettings: nil,
			},
		}}
	otherPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "otherPolicyNs",
			Name:      "otherPolicyName",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "secure-settings-secret"}},
			},
		}}

	secret, _, err := NewSettingsSecretWithVersion(es, false, nil, nil, nil, metadata.Metadata{})
	assert.NoError(t, err)

	secureSettings, err := getSecureSettings(secret)
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	err = setSecureSettings(&secret, policy.GetElasticsearchNamespacedSecureSettings())
	assert.NoError(t, err)
	secureSettings, err = getSecureSettings(secret)
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	err = setSecureSettings(&secret, otherPolicy.GetElasticsearchNamespacedSecureSettings())
	assert.NoError(t, err)
	secureSettings, err = getSecureSettings(secret)
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{{Namespace: otherPolicy.Namespace, SecretName: "secure-settings-secret"}}, secureSettings)
}

func parseSettings(t *testing.T, secret corev1.Secret) Settings {
	t.Helper()
	var settings Settings
	err := json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	assert.NoError(t, err)
	return settings
}
