// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnoughRedundancy(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "valid copy exists elsewhere")
}

func TestNothingToMigrate(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "no data on node A")
}

func TestOnlyCopyNeedsMigration(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		{Index: "index-1", Shard: 1, State: client.STARTED, Node: "B"},
		{Index: "index-1", Shard: 2, State: client.STARTED, Node: "C"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "no copy somewhere else")
}

func TestRelocationIsMigration(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.RELOCATING, Node: "A"},
		{Index: "index-1", Shard: 0, State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "await shard copy being relocated")
}

func TestCopyIsInitializing(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		{Index: "index-1", Shard: 0, State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "shard copy exists but is still initializing")
}

func TestIgnoresOtherMigrationCandidatesOtherCopyExists(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards, exclusionMap([]string{"A", "B"})), "valid copy exists in C")
}

func TestIgnoresOtherMigrationCandidatesNoOtherCopy(t *testing.T) {
	shards := []client.Shard{
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, exclusionMap([]string{"B", "C"})), "no valid copy all nodes are excluded")
}

func exclusionMap(exclusions []string) map[string]struct{} {
	excludedNodes := make(map[string]struct{})
	for _, n := range exclusions {
		excludedNodes[n] = struct{}{}
	}
	return excludedNodes
}

type mockClient struct {
	t    *testing.T
	seen []string
}

func (m *mockClient) ExcludeFromShardAllocation(context context.Context, nodes string) error {
	m.seen = append(m.seen, nodes)
	return nil
}

func (m *mockClient) getAndReset() []string {
	v := m.seen
	m.seen = []string{}
	return v
}

func TestMigrateData(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{
			name:  "no nodes to migrate",
			input: []string{},
			want:  "none_excluded",
		},
		{
			name:  "one node to migrate",
			input: []string{"test-node"},
			want:  "test-node",
		},
		{
			name:  "multiple node to migrate",
			input: []string{"test-node1", "test-node2"},
			want:  "test-node1,test-node2",
		},
	}

	for _, tt := range tests {
		esClient := &mockClient{t: t}
		err := setAllocationExcludes(esClient, tt.input)
		require.NoError(t, err)
		assert.Contains(t, esClient.getAndReset(), tt.want)
	}
}

func TestIsMigratingData(t *testing.T) {
	type args struct {
		state      observer.State
		podName    string
		exclusions []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "cluster state is nil",
			args: args{
				state:      observer.State{ClusterState: nil},
				podName:    "pod",
				exclusions: nil,
			},
			want: true,
		},
		{
			name: "cluster state is empty",
			args: args{
				state:      observer.State{ClusterState: &client.ClusterState{}},
				podName:    "pod",
				exclusions: nil,
			},
			want: true,
		},
		{
			name: "no data migration in progress",
			args: args{
				state: observer.State{ClusterState: &client.ClusterState{
					ClusterName: "name",
				}},
				podName:    "pod",
				exclusions: nil,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMigratingData(tt.args.state, tt.args.podName, tt.args.exclusions); got != tt.want {
				t.Errorf("IsMigratingData() = %v, want %v", got, tt.want)
			}
		})
	}
}
