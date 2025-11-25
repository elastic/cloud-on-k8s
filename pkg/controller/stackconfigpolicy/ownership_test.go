// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"encoding/json"
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
	tests := []struct {
		name     string
		secret   *corev1.Secret
		policy   policyv1alpha1.StackConfigPolicy
		validate func(t *testing.T, secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy)
	}{
		{
			name: "overwrites existing soft owner labels",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      "old-kind",
						reconciler.SoftOwnerNameLabel:      "old-policy",
						reconciler.SoftOwnerNamespaceLabel: "old-namespace",
						"existing-label":                   "existing-value",
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: "{}",
						"existing-annotation":              "existing-value",
					},
				},
			},
			policy: policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-policy",
					Namespace: "new-namespace",
				},
			},
			validate: func(t *testing.T, secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
				assert.NotNil(t, secret.Labels)
				assert.Equal(t, policyv1alpha1.Kind, secret.Labels[reconciler.SoftOwnerKindLabel])
				assert.Equal(t, "new-policy", secret.Labels[reconciler.SoftOwnerNameLabel])
				assert.Equal(t, "new-namespace", secret.Labels[reconciler.SoftOwnerNamespaceLabel])
				assert.Equal(t, "existing-value", secret.Labels["existing-label"])
				assert.Equal(t, "existing-value", secret.Annotations["existing-annotation"])
				assert.NotContains(t, secret.Annotations, reconciler.SoftOwnerRefsAnnotation)
			},
		},
		{
			name:   "returns nil for nil secret",
			secret: nil,
			validate: func(t *testing.T, secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
				assert.Nil(t, secret)
			},
		},
		{
			name:   "secret with nil labels and annotations",
			secret: &corev1.Secret{},
			validate: func(t *testing.T, secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
				assert.NotNil(t, secret)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setSingleSoftOwner(tt.secret, tt.policy)
			tt.validate(t, tt.secret, tt.policy)
		})
	}
}

//nolint:thelper
func Test_setMultipleSoftOwners(t *testing.T) {
	tests := []struct {
		name     string
		secret   *corev1.Secret
		policies []policyv1alpha1.StackConfigPolicy
		validate func(t *testing.T, secret *corev1.Secret, policies []policyv1alpha1.StackConfigPolicy, err error)
	}{
		{
			name: "removes single-owner labels and sets multi-owner annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
						reconciler.SoftOwnerNameLabel:      "old-single-policy",
						reconciler.SoftOwnerNamespaceLabel: "old-namespace",
						"existing-label":                   "existing-value",
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: "replaced-value",
						"existing-annotation":              "existing-value",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
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
				{
					// should be deduplicated
					ObjectMeta: metav1.ObjectMeta{
						Name:      "policy-2",
						Namespace: "namespace-2",
					},
				},
			},
			validate: func(t *testing.T, secret *corev1.Secret, policies []policyv1alpha1.StackConfigPolicy, err error) {
				require.NoError(t, err)

				// Verify single-owner labels are removed
				assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNameLabel)
				assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNamespaceLabel)

				// Verify kind label is still set
				assert.Equal(t, policyv1alpha1.Kind, secret.Labels[reconciler.SoftOwnerKindLabel])

				// Verify existing label is preserved
				assert.Equal(t, "existing-value", secret.Labels["existing-label"])

				// Verify existing annotation is preserved
				assert.Equal(t, "existing-value", secret.Annotations["existing-annotation"])

				// Verify multi-owner annotation is set with both policies
				ownerRefsJSON := secret.Annotations[reconciler.SoftOwnerRefsAnnotation]
				assert.NotEmpty(t, ownerRefsJSON)

				var ownerRefs map[string]struct{}
				err = json.Unmarshal([]byte(ownerRefsJSON), &ownerRefs)
				require.NoError(t, err)
				assert.EqualValues(t, map[string]struct{}{
					types.NamespacedName{Name: "policy-1", Namespace: "namespace-1"}.String(): {},
					types.NamespacedName{Name: "policy-2", Namespace: "namespace-2"}.String(): {},
				}, ownerRefs)
			},
		},
		{
			name:   "returns nil for nil secret",
			secret: nil,
			validate: func(t *testing.T, secret *corev1.Secret, policies []policyv1alpha1.StackConfigPolicy, err error) {
				assert.Nil(t, err)
				assert.Nil(t, secret)
				assert.Len(t, policies, 0)
			},
		},
		{
			name:   "secret with nil labels and annotations",
			secret: &corev1.Secret{},
			validate: func(t *testing.T, secret *corev1.Secret, policies []policyv1alpha1.StackConfigPolicy, err error) {
				assert.Nil(t, err)
				assert.NotNil(t, secret)
				assert.Len(t, policies, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setMultipleSoftOwners(tt.secret, tt.policies)
			tt.validate(t, tt.secret, tt.policies, err)
		})
	}
}

//nolint:thelper
func Test_removePolicySoftOwner(t *testing.T) {
	tests := []struct {
		name           string
		secret         *corev1.Secret
		policyToRemove types.NamespacedName
		validate       func(t *testing.T, secret *corev1.Secret, remainingCount int, err error)
	}{
		{
			name: "removes policy from multi-owner with remaining owners",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `{"namespace-1/policy-1":{},"namespace-2/policy-2":{},"namespace-3/policy-3":{}}`,
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "policy-2", Namespace: "namespace-2"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2, remainingCount)

				// Verify the annotation still exists with remaining owners
				ownerRefsJSON := secret.Annotations[reconciler.SoftOwnerRefsAnnotation]
				assert.NotEmpty(t, ownerRefsJSON)

				var ownerRefs map[string]struct{}
				err = json.Unmarshal([]byte(ownerRefsJSON), &ownerRefs)
				require.NoError(t, err)
				assert.Len(t, ownerRefs, 2)

				// Verify policy-2 was removed
				assert.EqualValues(t, map[string]struct{}{
					"namespace-1/policy-1": {},
					"namespace-3/policy-3": {},
				}, ownerRefs)
			},
		},
		{
			name: "removes last policy from multi-owner and cleans up annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `{"namespace-1/policy-1":{}}`,
						"other-annotation":                 "preserved",
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "policy-1", Namespace: "namespace-1"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, remainingCount)

				// Verify the annotation was removed
				assert.NotContains(t, secret.Annotations, reconciler.SoftOwnerRefsAnnotation)

				// Verify other annotations are preserved
				assert.Equal(t, "preserved", secret.Annotations["other-annotation"])
			},
		},
		{
			name: "removes matching single-owner and cleans up labels",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
						reconciler.SoftOwnerNameLabel:      "single-policy",
						reconciler.SoftOwnerNamespaceLabel: "single-namespace",
						"other-label":                      "preserved",
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "single-policy", Namespace: "single-namespace"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, remainingCount)

				// Verify all soft owner labels were removed
				assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNameLabel)
				assert.NotContains(t, secret.Labels, reconciler.SoftOwnerNamespaceLabel)

				// Verify other labels are preserved
				assert.Equal(t, "preserved", secret.Labels["other-label"])
			},
		},
		{
			name: "returns 1 when policy doesn't match single-owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
						reconciler.SoftOwnerNameLabel:      "existing-policy",
						reconciler.SoftOwnerNamespaceLabel: "existing-namespace",
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "different-policy", Namespace: "different-namespace"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 1, remainingCount)

				// Verify labels remain unchanged
				assert.Equal(t, policyv1alpha1.Kind, secret.Labels[reconciler.SoftOwnerKindLabel])
				assert.Equal(t, "existing-policy", secret.Labels[reconciler.SoftOwnerNameLabel])
				assert.Equal(t, "existing-namespace", secret.Labels[reconciler.SoftOwnerNamespaceLabel])
			},
		},
		{
			name:           "returns 0 for nil secret",
			secret:         nil,
			policyToRemove: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, remainingCount)
			},
		},
		{
			name: "returns 0 for non-owned secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"some-label": "some-value",
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, remainingCount)
			},
		},
		{
			name: "returns error for invalid JSON in annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `invalid-json`,
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.Error(t, err)
				assert.Equal(t, 0, remainingCount)
			},
		},
		{
			name: "returns 0 when removing non-existent policy from multi-owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `{"namespace-1/policy-1":{},"namespace-2/policy-2":{}}`,
					},
				},
			},
			policyToRemove: types.NamespacedName{Name: "non-existent", Namespace: "namespace-3"},
			validate: func(t *testing.T, secret *corev1.Secret, remainingCount int, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2, remainingCount)

				// Verify both original policies remain
				var ownerRefs map[string]struct{}
				err = json.Unmarshal([]byte(secret.Annotations[reconciler.SoftOwnerRefsAnnotation]), &ownerRefs)
				require.NoError(t, err)
				assert.Len(t, ownerRefs, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remainingCount, err := removePolicySoftOwner(tt.secret, tt.policyToRemove)
			tt.validate(t, tt.secret, remainingCount, err)
		})
	}
}

//nolint:thelper
func Test_isPolicySoftOwner(t *testing.T) {
	tests := []struct {
		name      string
		secret    *corev1.Secret
		policyNsn types.NamespacedName
		validate  func(t *testing.T, isOwner bool, err error)
	}{
		{
			name: "returns true when policy is owner in multi-owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `{"namespace-1/policy-1":{},"namespace-2/policy-2":{}}`,
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "policy-1", Namespace: "namespace-1"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.True(t, isOwner)
			},
		},
		{
			name: "returns false when policy is not owner in multi-owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `{"namespace-1/policy-1":{},"namespace-2/policy-2":{}}`,
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "policy-3", Namespace: "namespace-3"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.False(t, isOwner)
			},
		},
		{
			name: "returns true when policy matches single-owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
						reconciler.SoftOwnerNameLabel:      "single-policy",
						reconciler.SoftOwnerNamespaceLabel: "single-namespace",
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "single-policy", Namespace: "single-namespace"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.True(t, isOwner)
			},
		},
		{
			name: "returns false when policy doesn't match single-owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      policyv1alpha1.Kind,
						reconciler.SoftOwnerNameLabel:      "single-policy",
						reconciler.SoftOwnerNamespaceLabel: "single-namespace",
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "different-policy", Namespace: "different-namespace"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.False(t, isOwner)
			},
		},
		{
			name:      "returns false for nil secret",
			secret:    nil,
			policyNsn: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.False(t, isOwner)
			},
		},
		{
			name: "returns false for non-policy-owned secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"some-other-label": "some-value",
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.False(t, isOwner)
			},
		},
		{
			name: "returns error for invalid JSON in annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						reconciler.SoftOwnerRefsAnnotation: `invalid-json`,
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.Error(t, err)
				assert.False(t, isOwner)
			},
		},
		{
			name: "returns false when secret has kind label but no owner references",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
				},
			},
			policyNsn: types.NamespacedName{Name: "policy", Namespace: "namespace"},
			validate: func(t *testing.T, isOwner bool, err error) {
				require.NoError(t, err)
				assert.False(t, isOwner)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isOwner, err := isPolicySoftOwner(tt.secret, tt.policyNsn)
			tt.validate(t, isOwner, err)
		})
	}
}
