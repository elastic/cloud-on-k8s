// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
)

func newPolicy(version string) *autoopsv1alpha1.AutoOpsAgentPolicy {
	return &autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version: version,
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
				SecretName: "config-secret",
			},
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
		},
	}
}

func TestCheckSupportedVersion(t *testing.T) {
	tests := []struct {
		name              string
		version           string
		enterpriseEnabled bool
		wantErr           bool
	}{
		{
			name:              "enterprise license with 9.2.1 is allowed",
			version:           "9.2.1",
			enterpriseEnabled: true,
			wantErr:           false,
		},
		{
			name:              "enterprise license with 9.2.3 is allowed",
			version:           "9.2.3",
			enterpriseEnabled: true,
			wantErr:           false,
		},
		{
			name:              "enterprise license with 9.2.4 is allowed",
			version:           "9.2.4",
			enterpriseEnabled: true,
			wantErr:           false,
		},
		{
			name:              "enterprise license with 9.2.0 is rejected",
			version:           "9.2.0",
			enterpriseEnabled: true,
			wantErr:           true,
		},
		{
			name:              "enterprise license with 9.1.0 is rejected",
			version:           "9.1.0",
			enterpriseEnabled: true,
			wantErr:           true,
		},
		{
			name:              "non-enterprise with 9.2.4 is allowed",
			version:           "9.2.4",
			enterpriseEnabled: false,
			wantErr:           false,
		},
		{
			name:              "non-enterprise with 9.3.0 is allowed",
			version:           "9.3.0",
			enterpriseEnabled: false,
			wantErr:           false,
		},
		{
			name:              "non-enterprise with 9.2.3 is rejected",
			version:           "9.2.3",
			enterpriseEnabled: false,
			wantErr:           true,
		},
		{
			name:              "non-enterprise with 9.2.1 is rejected",
			version:           "9.2.1",
			enterpriseEnabled: false,
			wantErr:           true,
		},
		{
			name:              "non-enterprise with 9.1.0 is rejected",
			version:           "9.1.0",
			enterpriseEnabled: false,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := license.MockLicenseChecker{EnterpriseEnabled: tt.enterpriseEnabled}
			policy := newPolicy(tt.version)
			errs := checkSupportedVersion(context.Background(), policy, checker)
			if tt.wantErr {
				require.NotEmpty(t, errs, "expected validation error for version %s with enterprise=%v", tt.version, tt.enterpriseEnabled)
			} else {
				require.Empty(t, errs, "unexpected validation error for version %s with enterprise=%v", tt.version, tt.enterpriseEnabled)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name              string
		policy            *autoopsv1alpha1.AutoOpsAgentPolicy
		enterpriseEnabled bool
		wantErr           bool
	}{
		{
			name:              "valid policy with non-enterprise license",
			policy:            newPolicy("9.2.4"),
			enterpriseEnabled: false,
			wantErr:           false,
		},
		{
			name:              "valid policy with enterprise license at lower version",
			policy:            newPolicy("9.2.1"),
			enterpriseEnabled: true,
			wantErr:           false,
		},
		{
			name:              "version too low for non-enterprise",
			policy:            newPolicy("9.2.1"),
			enterpriseEnabled: false,
			wantErr:           true,
		},
		{
			name:              "version too low for enterprise",
			policy:            newPolicy("9.1.0"),
			enterpriseEnabled: true,
			wantErr:           true,
		},
		{
			name: "missing secret name",
			policy: func() *autoopsv1alpha1.AutoOpsAgentPolicy {
				p := newPolicy("9.2.4")
				p.Spec.AutoOpsRef.SecretName = ""
				return p
			}(),
			enterpriseEnabled: false,
			wantErr:           true,
		},
		{
			name: "missing resource selector",
			policy: func() *autoopsv1alpha1.AutoOpsAgentPolicy {
				p := newPolicy("9.2.4")
				p.Spec.ResourceSelector = metav1.LabelSelector{}
				return p
			}(),
			enterpriseEnabled: false,
			wantErr:           true,
		},
		{
			name: "name too long",
			policy: func() *autoopsv1alpha1.AutoOpsAgentPolicy {
				p := newPolicy("9.2.4")
				p.Name = "a-very-long-name-that-exceeds-the-maximum-allowed-length-for-a-kubernetes-resource-and-should-be-rejected-by-the-validating-webhook-which-is-really-really-really-really-really-really-really-really-really-really-long"
				return p
			}(),
			enterpriseEnabled: false,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := license.MockLicenseChecker{EnterpriseEnabled: tt.enterpriseEnabled}
			err := Validate(context.Background(), tt.policy, checker)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
