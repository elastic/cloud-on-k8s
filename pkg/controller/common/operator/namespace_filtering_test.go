// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestShouldManageNamespace(t *testing.T) {
	// Test namespace with labels
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				"env":  "production",
				"team": "platform",
			},
		},
	}

	// Another test namespace with different labels
	devNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dev-namespace",
			Labels: map[string]string{
				"env":  "development",
				"team": "platform",
			},
		},
	}

	// Namespace without relevant labels
	otherNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-namespace",
			Labels: map[string]string{
				"purpose": "testing",
			},
		},
	}

	tests := []struct {
		name                   string
		namespaceLabelSelector *metav1.LabelSelector
		namespace              string
		expectedResult         bool
		expectedError          bool
	}{
		{
			name:                   "No label selector - should manage all namespaces",
			namespaceLabelSelector: nil,
			namespace:              "test-namespace",
			expectedResult:         true,
			expectedError:          false,
		},
		{
			name: "Matching label selector - should manage namespace",
			namespaceLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"env": "production",
				},
			},
			namespace:      "test-namespace",
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "Non-matching label selector - should not manage namespace",
			namespaceLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"env": "production",
				},
			},
			namespace:      "dev-namespace",
			expectedResult: false,
			expectedError:  false,
		},
		{
			name: "Multiple label selectors - should match when all match",
			namespaceLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"env":  "production",
					"team": "platform",
				},
			},
			namespace:      "test-namespace",
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "Multiple label selectors - should not match when one doesn't match",
			namespaceLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"env":  "production",
					"team": "different",
				},
			},
			namespace:      "test-namespace",
			expectedResult: false,
			expectedError:  false,
		},
		{
			name: "Complex selector with matchExpressions",
			namespaceLabelSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "env",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"development", "staging"},
					},
				},
			},
			namespace:      "test-namespace",
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "Label selector with missing labels",
			namespaceLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"env": "production",
				},
			},
			namespace:      "other-namespace",
			expectedResult: false,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test namespaces
			fakeClient := k8s.NewFakeClient(testNamespace, devNamespace, otherNamespace)

			// Create parameters with the test label selector
			params := Parameters{
				NamespaceLabelSelector: tt.namespaceLabelSelector,
			}

			// Test the function
			result, err := params.ShouldManageNamespace(context.Background(), fakeClient, tt.namespace)

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestShouldManageNamespace_NonExistentNamespace(t *testing.T) {
	fakeClient := k8s.NewFakeClient()

	params := Parameters{
		NamespaceLabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"env": "production",
			},
		},
	}

	result, err := params.ShouldManageNamespace(context.Background(), fakeClient, "non-existent")

	assert.False(t, result)
	assert.Error(t, err)
}