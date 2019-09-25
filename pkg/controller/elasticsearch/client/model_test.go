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

func TestShards_fixNodeNames(t *testing.T) {
	tests := []struct {
		name     string
		shards   Shards
		expected Shards
	}{
		{
			name: "Fix shard names",
			shards: Shards{
				Shard{
					Index:    "data-integrity-check",
					Shard:    "1",
					State:    "RELOCATING",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-1 -> 10.56.2.33 8DqGuLtrSNyMfE2EfKNDgg test-mutation-less-nodes-sqn9-es-masterdata-0",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "2",
					State:    "RELOCATING",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-2 -> 10.56.2.33 8DqGuLtrSNyMfE2EfKNDgg test-mutation-less-nodes-sqn9-es-masterdata-0",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "0",
					State:    "STARTED",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-0",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "3",
					State:    "UNASSIGNED",
					NodeName: "",
				},
			},
			expected: Shards{
				Shard{
					Index:    "data-integrity-check",
					Shard:    "1",
					State:    "RELOCATING",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-1",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "2",
					State:    "RELOCATING",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-2",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "0",
					State:    "STARTED",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-0",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "3",
					State:    "UNASSIGNED",
					NodeName: "",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.shards.fixNodeNames()
			assert.Equal(t, tt.expected, tt.shards)
		})
	}
}
