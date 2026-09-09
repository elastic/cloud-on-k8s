// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGKELocalSSDOption(t *testing.T) {
	tests := []struct {
		name     string
		settings *GKESettings
		want     string
		wantErr  string
	}{
		{
			name:     "no local SSD",
			settings: &GKESettings{},
		},
		{
			name: "legacy local SSD",
			settings: &GKESettings{
				LocalSsdCount: 1,
			},
			want: "--local-ssd-count 1",
		},
		{
			name: "raw NVMe local SSD",
			settings: &GKESettings{
				LocalNvmeSsdBlock: true,
			},
			want: "--local-nvme-ssd-block count=1",
		},
		{
			name: "mutually exclusive modes",
			settings: &GKESettings{
				LocalSsdCount:     1,
				LocalNvmeSsdBlock: true,
			},
			wantErr: "localSsdCount and localNvmeSsdBlock are mutually exclusive",
		},
		{
			name: "negative count",
			settings: &GKESettings{
				LocalSsdCount: -1,
			},
			wantErr: "local SSD count must not be negative",
		},
		{
			name:    "missing settings",
			wantErr: "GKE settings must be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gkeLocalSSDOption(tt.settings)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGKECIPlanUsesRawNVMeLocalSSD(t *testing.T) {
	plansYAML, err := os.ReadFile("../config/plans.yml")
	require.NoError(t, err)

	var plans Plans
	require.NoError(t, yaml.Unmarshal(plansYAML, &plans))

	plan, err := choosePlan(plans.Plans, "gke-ci")
	require.NoError(t, err)
	require.Equal(t, "europe-west1", plan.Gke.Region)
	require.Equal(t, "n2d-standard-4", plan.MachineType)
	require.Zero(t, plan.Gke.LocalSsdCount)
	require.True(t, plan.Gke.LocalNvmeSsdBlock)
	require.Equal(t, "kubectl apply -k hack/deployer/config/local-disks-gke", plan.DiskSetup)

	option, err := gkeLocalSSDOption(plan.Gke)
	require.NoError(t, err)
	require.Equal(t, "--local-nvme-ssd-block count=1", option)
}
