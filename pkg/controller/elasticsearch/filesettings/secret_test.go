// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_newSettingsSecret(t *testing.T) {
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
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{"a": "b"}},
			},
		},
	}

	// no policy
	ss, err := NewSettingsSecret(nil, es, nil)
	assert.NoError(t, err)
	assert.Equal(t, "esNs", ss.Namespace)
	assert.Equal(t, "esName-es-file-settings", ss.Name)

	// policy
	ss, err = NewSettingsSecret(nil, es, &policy)
	assert.NoError(t, err)
	assert.Equal(t, "esNs", ss.Namespace)
	assert.Equal(t, "esName-es-file-settings", ss.Name)
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
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{"a": "b"}},
			},
		}}

	tick := time.Now().UnixNano()
	_, emptySettings := NewSettings()

	// no policy -> emptySettings
	ss, err := NewSettingsSecret(nil, es, nil)
	assert.NoError(t, err)
	assert.Equal(t, false, ss.hasChanged(emptySettings))
	assert.Greater(t, ss.version, tick)

	// policy without settings -> emptySettings
	_, sameSettings := NewSettings()
	err = sameSettings.updateState(es, policy)
	assert.NoError(t, err)
	assert.Equal(t, false, ss.hasChanged(sameSettings))

	// new policu -> settings changed
	_, newSettings := NewSettings()
	err = newSettings.updateState(es, otherPolicy)
	assert.NoError(t, err)
	assert.Equal(t, true, ss.hasChanged(newSettings))
}

func Test_SettingsSecret_getVersion(t *testing.T) {
	es := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}

	tick1 := time.Now().UnixNano()
	ss, err := NewSettingsSecret(nil, es, nil)
	tick2 := time.Now().UnixNano()
	assert.NoError(t, err)

	v := ss.GetVersion()
	assert.LessOrEqual(t, tick1, v)
	assert.LessOrEqual(t, v, tick2)
}

func Test_SettingsSecret_setSoftOwner_canBeOwnedBy(t *testing.T) {
	es := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}
	policy := policyv1alpha1.StackConfigPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind: policyv1alpha1.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "policyNs",
			Name:      "policyName",
		},
	}
	otherPolicy := policyv1alpha1.StackConfigPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind: policyv1alpha1.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "otherPolicyNs",
			Name:      "otherPolicyName",
		},
	}

	// empty settings can be owned by any policy
	ss, err := NewSettingsSecret(nil, es, nil)
	assert.NoError(t, err)
	_, canBeOwned := ss.CanBeOwnedBy(policy)
	assert.Equal(t, true, canBeOwned)
	_, canBeOwned = ss.CanBeOwnedBy(otherPolicy)
	assert.Equal(t, true, canBeOwned)

	// set a policy soft owner
	ss.SetSoftOwner(policy)
	_, canBeOwned = ss.CanBeOwnedBy(policy)
	assert.Equal(t, true, canBeOwned)
	_, canBeOwned = ss.CanBeOwnedBy(otherPolicy)
	assert.Equal(t, false, canBeOwned)

	// update the policy soft owner
	ss.SetSoftOwner(otherPolicy)
	_, canBeOwned = ss.CanBeOwnedBy(policy)
	assert.Equal(t, false, canBeOwned)
	_, canBeOwned = ss.CanBeOwnedBy(otherPolicy)
	assert.Equal(t, true, canBeOwned)
}

func Test_SettingsSecret_setSecureSettings_getSecureSettings(t *testing.T) {
	es := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "esNs",
			Name:      "esNs-es-file-settings",
			Annotations: map[string]string{
				"policy.k8s.elastic.co/secure-settings-secrets": "secure-settings-secret",
			},
		},
	}
	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "policyNs",
			Name:      "policyName",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			SecureSettings: nil,
		}}
	otherPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "otherPolicyNs",
			Name:      "otherPolicyName",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			SecureSettings: []commonv1.NamespacedSecretSource{{SecretName: "secure-settings-secret"}},
		}}

	client := k8s.NewFakeClient(&secret)

	ss, err := NewSettingsSecret(nil, es, nil)
	assert.NoError(t, err)

	secureSettings, err := ss.getSecureSettings()
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	err = ss.SetSecureSettings(context.Background(), client, policy)
	assert.NoError(t, err)
	secureSettings, err = ss.getSecureSettings()
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	err = ss.SetSecureSettings(context.Background(), client, otherPolicy)
	assert.NoError(t, err)
	secureSettings, err = ss.getSecureSettings()
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{{SecretName: "secure-settings-secret"}}, secureSettings)
}
