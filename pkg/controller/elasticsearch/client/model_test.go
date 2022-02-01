// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

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
		arg  RemoteClustersSettings
		want string
	}{
		{
			name: "Simple remote cluster",
			arg: RemoteClustersSettings{
				PersistentSettings: &SettingsGroup{
					Cluster: RemoteClusters{
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
			arg: RemoteClustersSettings{
				PersistentSettings: &SettingsGroup{
					Cluster: RemoteClusters{
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

func TestModel_License(t *testing.T) {
	tests := []struct {
		name    string
		license License
		want    string
	}{
		{
			name: "platinum",
			license: License{
				UID:                "304d04fe-c2d2-8774-cd34-7a71a4cc8c4d",
				Type:               "platinum",
				IssueDateInMillis:  1576000000000,
				ExpiryDateInMillis: 1590000000000,
				MaxNodes:           100,
				IssuedTo:           "ECK Test",
				Issuer:             "ECK Tester",
				StartDateInMillis:  1576000000000,
				Signature:          "AAA...xyz",
			},
			want: `{"uid":"304d04fe-c2d2-8774-cd34-7a71a4cc8c4d","type":"platinum","issue_date_in_millis":1576000000000,"expiry_date_in_millis":1590000000000,"max_nodes":100,"issued_to":"ECK Test","issuer":"ECK Tester","start_date_in_millis":1576000000000,"signature":"AAA...xyz"}`,
		},
		{
			name: "enterprise",
			license: License{
				UID:                "304d04fe-c2d2-8774-cd34-7a71a4cc8c4d",
				Type:               "enterprise",
				IssueDateInMillis:  1576000000000,
				ExpiryDateInMillis: 1590000000000,
				MaxResourceUnits:   100,
				IssuedTo:           "ECK Test",
				Issuer:             "ECK Tester",
				StartDateInMillis:  1576000000000,
				Signature:          "AAA...xyz",
			},
			want: `{"uid":"304d04fe-c2d2-8774-cd34-7a71a4cc8c4d","type":"enterprise","issue_date_in_millis":1576000000000,"expiry_date_in_millis":1590000000000,"max_resource_units":100,"issued_to":"ECK Test","issuer":"ECK Tester","start_date_in_millis":1576000000000,"signature":"AAA...xyz"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json, err := json.Marshal(tt.license)
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

func TestShutdownResponse(t *testing.T) {
	nodeShudownSample := `{
	"nodes": [{
		"node_id": "PQKHA4xCQd2xErO2fUK-hg",
		"type": "REMOVE",
		"reason": "2331481",
		"shutdown_startedmillis": 1643648932189,
		"status": "IN_PROGRESS",
		"shard_migration": {
			"status": "IN_PROGRESS",
			"shard_migrations_remaining": 1
		},
		"persistent_tasks": {
			"status": "COMPLETE"
		},
		"plugins": {
			"status": "COMPLETE"
		}
	}]
}`
	expected := ShutdownResponse{Nodes: []NodeShutdown{
		{
			NodeID:                "PQKHA4xCQd2xErO2fUK-hg",
			Type:                  "REMOVE",
			Reason:                "2331481",
			ShutdownStartedMillis: 1643648932189,
			Status:                ShutdownInProgress,
			ShardMigration: ShardMigration{
				Status:                   ShutdownInProgress,
				ShardMigrationsRemaining: 1,
				Explanation:              "",
			},
			PersistentTasks: PersistentTasks{
				Status: ShutdownComplete,
			},
			Plugins: Plugins{
				Status: ShutdownComplete,
			},
		},
	},
	}

	var actual ShutdownResponse
	require.NoError(t, json.Unmarshal([]byte(nodeShudownSample), &actual))
	require.Equal(t, expected, actual)
}
