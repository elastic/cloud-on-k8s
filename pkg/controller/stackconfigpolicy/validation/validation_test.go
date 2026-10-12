// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
)

const operatorNamespace = "operator-ns"

func mkSCP(ns string) *policyv1alpha1.StackConfigPolicy {
	return &policyv1alpha1.StackConfigPolicy{
		Name:      "test-policy",
		Namespace: ns,
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

func TestValidate_ReservedRoleMappingNames(t *testing.T) {
	withRoleMappings := func(names ...string) *policyv1alpha1.StackConfigPolicy {
		p := mkSCP("default")
		data := map[string]any{}
		for _, name := range names {
			data[name] = map[string]any{"enabled": true}
		}
		p.Spec.Elasticsearch.SecurityRoleMappings = &commonv1.Config{Data: data}
		return p
	}
	reserved := commonv1.ECKPKIRoleMappingPrefix + "corporate"
	v := &webhookValidator{operatorNamespace: operatorNamespace}

	tests := []struct {
		name     string
		validate func() error
		wantErr  bool
	}{
		{
			name: "admission create rejects a reserved name",
			validate: func() error {
				_, err := v.ValidateCreate(t.Context(), withRoleMappings(reserved))
				return err
			},
			wantErr: true,
		},
		{
			name: "admission update accepts a reserved name already in the old policy",
			validate: func() error {
				_, err := v.ValidateUpdate(t.Context(), withRoleMappings(reserved), withRoleMappings(reserved, "custom_mapping"))
				return err
			},
		},
		{
			name: "admission update rejects a newly added reserved name",
			validate: func() error {
				_, err := v.ValidateUpdate(t.Context(), withRoleMappings("custom_mapping"), withRoleMappings(reserved, "custom_mapping"))
				return err
			},
			wantErr: true,
		},
		{
			name: "controller-side validation accepts a reserved name so existing policies keep being applied",
			validate: func() error {
				_, err := Validate(withRoleMappings(reserved), operatorNamespace)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.validate()
			require.Equal(t, tc.wantErr, err != nil, "unexpected error state: %v", err)
		})
	}
}
