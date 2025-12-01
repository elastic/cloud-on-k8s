// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
)

//nolint:thelper
func Test_setSingleSoftOwner(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}
	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "policy-namespace",
		},
	}

	setSingleSoftOwner(secret, policy)

	assert.Equal(t, policyv1alpha1.Kind, secret.Labels[reconciler.SoftOwnerKindLabel])
	assert.Equal(t, "test-policy", secret.Labels[reconciler.SoftOwnerNameLabel])
	assert.Equal(t, "policy-namespace", secret.Labels[reconciler.SoftOwnerNamespaceLabel])
}

func Test_setMultipleSoftOwners(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}
	policies := []policyv1alpha1.StackConfigPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-1",
				Namespace: "namespace-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-2",
				Namespace: "namespace-2",
			},
		},
	}

	err := setMultipleSoftOwners(secret, policies)
	require.NoError(t, err)

	assert.Equal(t, policyv1alpha1.Kind, secret.Labels[reconciler.SoftOwnerKindLabel])
	assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNameLabel)
	assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNamespaceLabel)
	assert.NotEmpty(t, secret.Annotations[reconciler.SoftOwnerRefsAnnotation])
}

func Test_isPolicySoftOwner(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
			Labels: map[string]string{
				reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
				reconciler.SoftOwnerNameLabel:      "test-policy",
				reconciler.SoftOwnerNamespaceLabel: "policy-namespace",
			},
		},
	}

	isOwner, err := isPolicySoftOwner(secret, types.NamespacedName{
		Namespace: "policy-namespace",
		Name:      "test-policy",
	})
	require.NoError(t, err)
	assert.True(t, isOwner)

	isOwner, err = isPolicySoftOwner(secret, types.NamespacedName{
		Namespace: "different-namespace",
		Name:      "different-policy",
	})
	require.NoError(t, err)
	assert.False(t, isOwner)
}

func Test_removePolicySoftOwner(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
			Labels: map[string]string{
				reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
				reconciler.SoftOwnerNameLabel:      "test-policy",
				reconciler.SoftOwnerNamespaceLabel: "policy-namespace",
			},
		},
	}

	remainingCount, err := removePolicySoftOwner(secret, types.NamespacedName{
		Namespace: "policy-namespace",
		Name:      "test-policy",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, remainingCount)
	assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNameLabel)
	assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNamespaceLabel)
}
