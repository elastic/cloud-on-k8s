// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

// fakeVaultClient is an in-memory vault.Client that returns the same data for any path and records the paths read.
type fakeVaultClient struct {
	data  map[string]any
	paths []string
}

func (c *fakeVaultClient) Read(path string) (*api.Secret, error) {
	c.paths = append(c.paths, path)
	return &api.Secret{Data: c.data}, nil
}

func TestOCPDriver_ensurePullSecret(t *testing.T) {
	const pullSecret = `{"auths":{"quay.io":{"auth":"Zm9v"}}}`
	vaultPath := "secret/ci/elastic-cloud-on-k8s/" + OCPVaultPath

	tests := []struct {
		name           string
		planPullSecret string
		vaultData      map[string]any
		want           string
		wantVaultPaths []string
		wantErr        string
	}{
		{
			name:           "pull secret from plan is used without reading vault",
			planPullSecret: pullSecret,
			vaultData:      map[string]any{OCPPullSecretFieldName: "from-vault"},
			want:           pullSecret,
		},
		{
			name:           "pull secret from plan is trimmed",
			planPullSecret: " \t" + pullSecret + "\n\n",
			want:           pullSecret,
		},
		{
			name:           "pull secret is read from vault when not in plan",
			vaultData:      map[string]any{OCPPullSecretFieldName: pullSecret},
			want:           pullSecret,
			wantVaultPaths: []string{vaultPath},
		},
		{
			name:           "pull secret from vault is trimmed",
			vaultData:      map[string]any{OCPPullSecretFieldName: pullSecret + "\n"},
			want:           pullSecret,
			wantVaultPaths: []string{vaultPath},
		},
		{
			name:           "whitespace-only pull secret in plan is replaced by vault",
			planPullSecret: " \n",
			vaultData:      map[string]any{OCPPullSecretFieldName: pullSecret},
			want:           pullSecret,
			wantVaultPaths: []string{vaultPath},
		},
		{
			name:           "error when field is missing in vault",
			vaultData:      map[string]any{"other-field": "value"},
			wantVaultPaths: []string{vaultPath},
			wantErr:        "field " + OCPPullSecretFieldName + " not found at " + vaultPath,
		},
		{
			name:           "error when field in vault is not a string",
			vaultData:      map[string]any{OCPPullSecretFieldName: 42},
			wantVaultPaths: []string{vaultPath},
			wantErr:        "is not a string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VAULT_ROOT_PATH", "")
			c := &fakeVaultClient{data: tt.vaultData}
			d := &OCPDriver{
				plan:        Plan{Ocp: &OCPSettings{PullSecret: tt.planPullSecret}},
				vaultClient: c,
			}

			err := d.ensurePullSecret()

			require.Equal(t, tt.wantVaultPaths, c.paths)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, d.plan.Ocp.PullSecret)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, d.plan.Ocp.PullSecret)
		})
	}
}

func Test_buildInfraIDRegexp(t *testing.T) {
	re := buildInfraIDRegexp("eck-e2e")
	tests := []struct {
		name        string
		input       string
		wantMatch   bool
		wantCluster string
	}{
		{
			name:        "standard CI infra ID with build number",
			input:       "eck-e2e-ocp-tfqh-4081-jkj45",
			wantMatch:   true,
			wantCluster: "eck-e2e-ocp-tfqh-4081",
		},
		{
			name:        "smoke test infra ID without build number",
			input:       "eck-e2e-ocp-testsmoke-gf6fg",
			wantMatch:   true,
			wantCluster: "eck-e2e-ocp-testsmoke",
		},
		{
			name:        "simple infra ID",
			input:       "eck-e2e-ocp-ci-ab1c2",
			wantMatch:   true,
			wantCluster: "eck-e2e-ocp-ci",
		},
		{
			name:      "internal OCP component SA (double dash)",
			input:     "eck-e2e-ocp--cloud-crede-lhtvg",
			wantMatch: false,
		},
		{
			name:      "internal OCP component SA (openshift-ingress)",
			input:     "eck-e2e-ocp--openshift-i-tn6nr",
			wantMatch: false,
		},
		{
			name:      "wrong prefix",
			input:     "ocp4-ci-1064-n78l5",
			wantMatch: false,
		},
		{
			name:      "suffix too short",
			input:     "eck-e2e-ocp-ci-ab1",
			wantMatch: false,
		},
		{
			name:      "suffix too long",
			input:     "eck-e2e-ocp-ci-ab1c2d",
			wantMatch: false,
		},
		{
			name:      "uppercase chars in suffix",
			input:     "eck-e2e-ocp-ci-AB1C2",
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := re.FindStringSubmatch(tt.input)
			if tt.wantMatch && len(matches) < 2 {
				t.Errorf("expected match for %q, got none", tt.input)
			}
			if !tt.wantMatch && len(matches) >= 2 {
				t.Errorf("expected no match for %q, got %v", tt.input, matches)
			}
			if tt.wantMatch && len(matches) >= 2 && matches[1] != tt.wantCluster {
				t.Errorf("cluster name: got %q, want %q", matches[1], tt.wantCluster)
			}
		})
	}
}

func Test_ocpServiceAccountRE(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMatch   bool
		wantInfraID string
	}{
		{
			name:        "master SA",
			input:       "eck-e2e-ocp-tfqh-4081-jkj45-m@elastic-cloud-dev.iam.gserviceaccount.com",
			wantMatch:   true,
			wantInfraID: "eck-e2e-ocp-tfqh-4081-jkj45",
		},
		{
			name:        "worker SA",
			input:       "eck-e2e-ocp-tfqh-4081-jkj45-w@elastic-cloud-dev.iam.gserviceaccount.com",
			wantMatch:   true,
			wantInfraID: "eck-e2e-ocp-tfqh-4081-jkj45",
		},
		{
			name:        "bootstrap SA",
			input:       "eck-e2e-ocp-tfqh-4081-jkj45-b@elastic-cloud-dev.iam.gserviceaccount.com",
			wantMatch:   true,
			wantInfraID: "eck-e2e-ocp-tfqh-4081-jkj45",
		},
		{
			name:        "truncated SA (no role suffix, infra ID >30 chars)",
			input:       "eck-e2e-ocp-tfqh-4081-jkj45@elastic-cloud-dev.iam.gserviceaccount.com",
			wantMatch:   true,
			wantInfraID: "eck-e2e-ocp-tfqh-4081-jkj45",
		},
		{
			// Internal OCP component SAs (e.g. openshift-ingress) are matched by this regex,
			// capturing the full SA name as the "infra ID". They are filtered out downstream
			// by buildInfraIDRegexp, which rejects names containing double dashes.
			name:        "internal component SA (openshift-ingress)",
			input:       "eck-e2e-ocp-tfqh-4081-jkj45-openshift-ingre@elastic-cloud-dev.iam.gserviceaccount.com",
			wantMatch:   true,
			wantInfraID: "eck-e2e-ocp-tfqh-4081-jkj45-openshift-ingre",
		},
		{
			name:      "no @ sign",
			input:     "eck-e2e-ocp-tfqh-4081-jkj45-m",
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ocpServiceAccountRE.FindStringSubmatch(tt.input)
			if tt.wantMatch && len(matches) < 2 {
				t.Errorf("expected match for %q, got none", tt.input)
			}
			if !tt.wantMatch && len(matches) >= 2 {
				t.Errorf("expected no match for %q, got %v", tt.input, matches)
			}
			if tt.wantMatch && len(matches) >= 2 && matches[1] != tt.wantInfraID {
				t.Errorf("infra ID: got %q, want %q", matches[1], tt.wantInfraID)
			}
		})
	}
}
