// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_RemoteCluster(t *testing.T) {
	tests := []struct {
		name string
		arg  Settings
		want string
	}{
		{
			name: "Simple remote cluster",
			arg: Settings{
				PersistentSettings: &SettingsGroup{
					Cluster: Cluster{
						RemoteClusters: map[string]RemoteCluster{
							"leader": {
								Seeds: []string{"127.0.0.1:9300"},
							},
						},
					},
				},
			},
			want: `{"persistent":{"cluster":{"remote":{"leader":{"seeds":["127.0.0.1:9300"]}}}}}`,
		},
		{
			name: "Deleted remote cluster",
			arg: Settings{
				PersistentSettings: &SettingsGroup{
					Cluster: Cluster{
						RemoteClusters: map[string]RemoteCluster{
							"leader": {
								Seeds: nil,
							},
						},
					},
				},
			},
			want: `{"persistent":{"cluster":{"remote":{"leader":{"seeds":null}}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json, err := json.Marshal(tt.arg)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, string(json))
		})
	}
}

func TestClusterRoutingAllocation(t *testing.T) {
	clusterSettingsSample := `{"persistent":{},"transient":{"cluster":{"routing":{"allocation":{"enable":"none","exclude":{"_name":"excluded"}}}}}}`
	expected := ClusterRoutingAllocation{Transient: AllocationSettings{Cluster: ClusterRoutingSettings{Routing: RoutingSettings{Allocation: RoutingAllocationSettings{Enable: "none", Exclude: AllocationExclude{Name: "excluded"}}}}}}

	var settings ClusterRoutingAllocation
	require.NoError(t, json.Unmarshal([]byte(clusterSettingsSample), &settings))
	require.Equal(t, expected, settings)
	require.Equal(t, false, settings.Transient.IsShardsAllocationEnabled())
}

func TestLicenseUpdateResponse_IsSuccess(t *testing.T) {
	type fields struct {
		Acknowledged  bool
		LicenseStatus string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "success: valid & ack'ed",
			fields: fields{
				Acknowledged:  true,
				LicenseStatus: "valid",
			},
			want: true,
		},
		{
			name: "no success: not valid",
			fields: fields{
				Acknowledged:  true,
				LicenseStatus: "invalid",
			},
			want: false,
		},
		{
			name: "no success: not ack'ed",
			fields: fields{
				Acknowledged:  false,
				LicenseStatus: "valid",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := LicenseUpdateResponse{
				Acknowledged:  tt.fields.Acknowledged,
				LicenseStatus: tt.fields.LicenseStatus,
			}
			if got := lr.IsSuccess(); got != tt.want {
				t.Errorf("IsSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}
