// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
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

	version := time.Now().UnixNano()
	ss, err := NewSettingsSecret(version, nil, es, nil)
	assert.NoError(t, err)
	assert.Equal(t, "esNs", ss.Namespace)
	assert.Equal(t, "esName-es-file-settings", ss.Name)
	assert.Equal(t, 0, len(ss.Settings.State.ClusterSettings.Data))
	assert.Equal(t, version, ss.Version)

	// policy
	version2 := time.Now().UnixNano()
	ss, err = NewSettingsSecret(version2, nil, es, &policy)
	assert.NoError(t, err)
	assert.Equal(t, "esNs", ss.Namespace)
	assert.Equal(t, "esName-es-file-settings", ss.Name)
	assert.Equal(t, 1, len(ss.Settings.State.ClusterSettings.Data))
	assert.Equal(t, version2, ss.Version)
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

	version := time.Now().UnixNano()
	_, expectedEmptySettings := NewSettings(version)

	// no policy -> emptySettings
	ss, err := NewSettingsSecret(version, nil, es, nil)
	assert.NoError(t, err)
	assert.Equal(t, false, ss.hasChanged(expectedEmptySettings))
	assert.Equal(t, version, ss.Version)

	// policy without settings -> emptySettings
	_, sameSettings := NewSettings(version)
	err = sameSettings.updateState(es, policy)
	assert.NoError(t, err)
	assert.Equal(t, false, ss.hasChanged(sameSettings))
	assert.Equal(t, strconv.FormatInt(version, 10), sameSettings.Metadata.Version)

	// new policy -> settings changed
	newVersion := time.Now().UnixNano()
	_, newSettings := NewSettings(newVersion)
	err = newSettings.updateState(es, otherPolicy)
	assert.NoError(t, err)
	assert.Equal(t, true, ss.hasChanged(newSettings))
	assert.Equal(t, strconv.FormatInt(newVersion, 10), newSettings.Metadata.Version)
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
	ss, err := NewSettingsSecret(time.Now().UnixNano(), nil, es, nil)
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
			SecureSettings: []commonv1.SecretSource{{SecretName: "secure-settings-secret"}},
		}}

	ss, err := NewSettingsSecret(time.Now().UnixNano(), nil, es, nil)
	assert.NoError(t, err)

	secureSettings, err := ss.getSecureSettings()
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	err = ss.SetSecureSettings(policy)
	assert.NoError(t, err)
	secureSettings, err = ss.getSecureSettings()
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{}, secureSettings)

	err = ss.SetSecureSettings(otherPolicy)
	assert.NoError(t, err)
	secureSettings, err = ss.getSecureSettings()
	assert.NoError(t, err)
	assert.Equal(t, []commonv1.NamespacedSecretSource{{Namespace: otherPolicy.Namespace, SecretName: "secure-settings-secret"}}, secureSettings)
}
