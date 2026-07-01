// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
)

const operatorNamespace = "operator-ns"

func mkSCP(ns string) *policyv1alpha1.StackConfigPolicy {
	return &policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: ns,
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
			},
		},
	}
}

func TestValidate_VariablesFrom(t *testing.T) {
	tests := []struct {
		name    string
		policy  *policyv1alpha1.StackConfigPolicy
		wantErr bool
	}{
		{
			name:   "no sources: no error",
			policy: mkSCP("default"),
		},
		{
			name: "source without namespace defaults to policy namespace: allowed",
			policy: func() *policyv1alpha1.StackConfigPolicy {
				p := mkSCP("default")
				p.Spec.VariablesFrom = []policyv1alpha1.VariableSource{
					{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "my-cm"},
					{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "my-secret"},
				}
				return p
			}(),
		},
		{
			name: "explicit same namespace as policy: allowed",
			policy: func() *policyv1alpha1.StackConfigPolicy {
				p := mkSCP("default")
				p.Spec.VariablesFrom = []policyv1alpha1.VariableSource{
					{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "my-secret", Namespace: "default"},
				}
				return p
			}(),
		},
		{
			name: "cross-namespace source rejected for namespace-scoped policy",
			policy: func() *policyv1alpha1.StackConfigPolicy {
				p := mkSCP("default")
				p.Spec.VariablesFrom = []policyv1alpha1.VariableSource{
					{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "my-secret", Namespace: "other-ns"},
				}
				return p
			}(),
			wantErr: true,
		},
		{
			name: "cross-namespace source allowed for policy in operator namespace",
			policy: func() *policyv1alpha1.StackConfigPolicy {
				p := mkSCP(operatorNamespace)
				p.Spec.VariablesFrom = []policyv1alpha1.VariableSource{
					{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "my-secret", Namespace: "other-ns"},
				}
				return p
			}(),
		},
		{
			name: "first of multiple sources cross-namespace causes error",
			policy: func() *policyv1alpha1.StackConfigPolicy {
				p := mkSCP("default")
				p.Spec.VariablesFrom = []policyv1alpha1.VariableSource{
					{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "my-cm", Namespace: "other-ns"},
					{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "my-secret"},
				}
				return p
			}(),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(tc.policy, operatorNamespace)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
