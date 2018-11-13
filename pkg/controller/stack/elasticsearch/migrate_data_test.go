package elasticsearch

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
)

func TestEnoughRedundancy(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards), "valid copy exists elsewhere")
}

func TestNothingToMigrate(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards), "no data on node A")
}

func TestOnlyCopyNeedsMigration(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 1, State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: 2, State: client.STARTED, Node: "C"},
	}
	assert.True(t, nodeIsMigratingData("A", shards), "no copy somewhere else")
}

func TestRelocationIsMigration(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.RELOCATING, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards), "await shard copy being relocated")
}

func TestCopyIsInitializing(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: 0, State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: 0, State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards), "shard copy exists but is still initializing")
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
	now := time.Now()
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{
			name:  "no indices to migrate",
			input: []string{},
			want:  "none_excluded",
		},
		{
			name:  "one index to migrate",
			input: []string{"test-index"},
			want:  "test-index," + strconv.FormatInt(now.Unix(), 10),
		},
		{
			name:  "multiple indices to migrate",
			input: []string{"test-index1", "test-index2"},
			want:  "test-index1,test-index2," + strconv.FormatInt(now.Unix(), 10),
		},

	}

	for _, tt := range tests {
		client := &mockClient{t: t,}
		setAllocationExcludes(client, tt.input, now)
		assert.Contains(t, client.getAndReset(), tt.want)
	}
}
