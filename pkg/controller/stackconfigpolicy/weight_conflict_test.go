// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
)

func TestCheckWeightConflicts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1alpha1.AddToScheme(scheme))

	tests := []struct {
		name              string
		currentPolicy     *policyv1alpha1.StackConfigPolicy
		existingPolicies  []policyv1alpha1.StackConfigPolicy
		operatorNamespace string
		expectError       bool
		errorContains     string
	}{
		{
			name: "no conflicts - different weights",
			currentPolicy: &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "default"},
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Weight: 10,
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			existingPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "default"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Weight: 20,
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "elasticsearch"},
						},
					},
				},
			},
			operatorNamespace: "elastic-system",
			expectError:       false,
		},
		{
			name: "conflict - same weight, overlapping selectors",
			currentPolicy: &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "default"},
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Weight: 10,
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			existingPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "default"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Weight: 10,
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "elasticsearch"},
						},
					},
				},
			},
			operatorNamespace: "elastic-system",
			expectError:       true,
			errorContains:     "weight conflict detected",
		},
		{
			name: "no conflict - same weight, disjoint selectors",
			currentPolicy: &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "default"},
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Weight: 10,
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			existingPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "default"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Weight: 10,
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "kibana"},
						},
					},
				},
			},
			operatorNamespace: "elastic-system",
			expectError:       false,
		},
		{
			name: "no conflict - same weight, different namespaces (non-operator)",
			currentPolicy: &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "namespace1"},
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Weight: 10,
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			existingPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "namespace2"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Weight: 10,
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "elasticsearch"},
						},
					},
				},
			},
			operatorNamespace: "elastic-system",
			expectError:       false,
		},
		{
			name: "conflict - same weight, operator namespace policy",
			currentPolicy: &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "elastic-system"},
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Weight: 10,
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			existingPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "default"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Weight: 10,
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "elasticsearch"},
						},
					},
				},
			},
			operatorNamespace: "elastic-system",
			expectError:       true,
			errorContains:     "weight conflict detected",
		},
		{
			name: "no conflict - empty selectors but different namespaces",
			currentPolicy: &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "namespace1"},
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Weight:           10,
					ResourceSelector: metav1.LabelSelector{},
				},
			},
			existingPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "namespace2"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Weight:           10,
						ResourceSelector: metav1.LabelSelector{},
					},
				},
			},
			operatorNamespace: "elastic-system",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]client.Object, len(tt.existingPolicies))
			for i := range tt.existingPolicies {
				objs[i] = &tt.existingPolicies[i]
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				Build()

			r := &ReconcileStackConfigPolicy{
				Client: fakeClient,
				params: operator.Parameters{
					OperatorNamespace: tt.operatorNamespace,
				},
				recorder:       record.NewFakeRecorder(10),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				dynamicWatches: watches.NewDynamicWatches(),
			}

			err := r.checkWeightConflicts(context.Background(), tt.currentPolicy)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSelectorsCouldOverlap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1alpha1.AddToScheme(scheme))

	r := &ReconcileStackConfigPolicy{}

	tests := []struct {
		name      string
		selector1 metav1.LabelSelector
		selector2 metav1.LabelSelector
		expected  bool
	}{
		{
			name:      "both empty selectors",
			selector1: metav1.LabelSelector{},
			selector2: metav1.LabelSelector{},
			expected:  true,
		},
		{
			name:      "one empty selector",
			selector1: metav1.LabelSelector{},
			selector2: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			expected:  true,
		},
		{
			name:      "same labels",
			selector1: metav1.LabelSelector{MatchLabels: map[string]string{"app": "elasticsearch"}},
			selector2: metav1.LabelSelector{MatchLabels: map[string]string{"app": "elasticsearch"}},
			expected:  true,
		},
		{
			name:      "different labels",
			selector1: metav1.LabelSelector{MatchLabels: map[string]string{"app": "elasticsearch"}},
			selector2: metav1.LabelSelector{MatchLabels: map[string]string{"app": "kibana"}},
			expected:  false,
		},
		{
			name:      "overlapping labels",
			selector1: metav1.LabelSelector{MatchLabels: map[string]string{"app": "elasticsearch", "env": "prod"}},
			selector2: metav1.LabelSelector{MatchLabels: map[string]string{"app": "elasticsearch", "version": "8.0"}},
			expected:  true,
		},
		{
			name:      "completely disjoint labels",
			selector1: metav1.LabelSelector{MatchLabels: map[string]string{"app": "elasticsearch", "env": "prod"}},
			selector2: metav1.LabelSelector{MatchLabels: map[string]string{"app": "kibana", "env": "test"}},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector1, err := metav1.LabelSelectorAsSelector(&tt.selector1)
			require.NoError(t, err)

			selector2, err := metav1.LabelSelectorAsSelector(&tt.selector2)
			require.NoError(t, err)

			result := r.selectorsCouldOverlap(selector1, selector2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNamespacesCouldOverlap(t *testing.T) {
	r := &ReconcileStackConfigPolicy{
		params: operator.Parameters{
			OperatorNamespace: "elastic-system",
		},
	}

	tests := []struct {
		name     string
		ns1      string
		ns2      string
		expected bool
	}{
		{
			name:     "same namespace",
			ns1:      "default",
			ns2:      "default",
			expected: true,
		},
		{
			name:     "different non-operator namespaces",
			ns1:      "namespace1",
			ns2:      "namespace2",
			expected: false,
		},
		{
			name:     "one is operator namespace",
			ns1:      "elastic-system",
			ns2:      "default",
			expected: true,
		},
		{
			name:     "other is operator namespace",
			ns1:      "default",
			ns2:      "elastic-system",
			expected: true,
		},
		{
			name:     "both are operator namespace",
			ns1:      "elastic-system",
			ns2:      "elastic-system",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.namespacesCouldOverlap(tt.ns1, tt.ns2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

