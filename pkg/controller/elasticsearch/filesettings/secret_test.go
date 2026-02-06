// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
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
	secret, reconciledVersion, err := newSettingsSecret(expectedVersion, es, nil, nil, nil, metadata.Metadata{})
	assert.NoError(t, err)
	assert.Equal(t, "esNs", secret.Namespace)
	assert.Equal(t, "esName-es-file-settings", secret.Name)
	assert.Equal(t, 0, len(parseSettings(t, secret).State.ClusterSettings.Data))
	assert.Equal(t, expectedVersion, reconciledVersion)

	// policy
	expectedVersion = int64(2)
	secret, reconciledVersion, err = newSettingsSecret(expectedVersion, es, &secret, &policy.Spec.Elasticsearch, policy.GetElasticsearchNamespacedSecureSettings(), metadata.Metadata{})
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
	expectedEmptySettings := NewEmptySettings(expectedVersion)

	// no policy -> emptySettings
	secret, reconciledVersion, err := newSettingsSecret(expectedVersion, es, nil, nil, nil, metadata.Metadata{})
	assert.NoError(t, err)
	assert.Equal(t, false, hasChanged(secret, expectedEmptySettings))
	assert.Equal(t, expectedVersion, reconciledVersion)

	// policy without settings -> emptySettings
	sameSettings := NewEmptySettings(expectedVersion)
	err = sameSettings.updateState(es, policy.Spec.Elasticsearch)
	assert.NoError(t, err)
	assert.Equal(t, false, hasChanged(secret, sameSettings))
	assert.Equal(t, strconv.FormatInt(expectedVersion, 10), sameSettings.Metadata.Version)

	// new policy -> settings changed
	newVersion := int64(2)
	newSettings := NewEmptySettings(newVersion)

	err = newSettings.updateState(es, otherPolicy.Spec.Elasticsearch)
	assert.NoError(t, err)
	assert.Equal(t, true, hasChanged(secret, newSettings))
	assert.Equal(t, strconv.FormatInt(newVersion, 10), newSettings.Metadata.Version)
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

	secret, _, err := NewSettingsSecretWithVersion(es, nil, nil, nil, metadata.Metadata{})
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
