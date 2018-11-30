package support

import (
	"context"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/assert"
)

func TestEnoughRedundancy(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "valid copy exists elsewhere")
}

func TestNothingToMigrate(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "no data on node A")
}

func TestOnlyCopyNeedsMigration(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 1, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 2, State: client.STARTED, Node: "C"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "no copy somewhere else")
}

func TestRelocationIsMigration(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.RELOCATING, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "await shard copy being relocated")
}

func TestCopyIsInitializing(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards, make(map[string]struct{})), "shard copy exists but is still initializing")
}

func TestIgnoresOtherMigrationCandidatesOtherCopyExists(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards, exclusionMap([]string{"A", "B"})), "valid copy exists in C")
}

func TestIgnoresOtherMigrationCandidatesNoOtherCopy(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
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
	now := time.Date(2018, 11, 15, 0, 9, 0, 0, time.UTC) //1542240540
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
			want:  "test-node,1542240540",
		},
		{
			name:  "multiple node to migrate",
			input: []string{"test-node1", "test-node2"},
			want:  "test-node1,test-node2,1542240540",
		},
	}

	for _, tt := range tests {
		esClient := &mockClient{t: t}
		setAllocationExcludes(esClient, tt.input, now)
		assert.Contains(t, esClient.getAndReset(), tt.want)
	}
}
