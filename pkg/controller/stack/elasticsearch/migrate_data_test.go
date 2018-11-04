package elasticsearch

import (
	"testing"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
)

func TestEnoughRedundancy(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards), "valid copy exists elsewhere")
}

func TestNothingToMigrate(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "C"},
	}
	assert.False(t, nodeIsMigratingData("A", shards), "no data on node A")
}

func TestOnlyCopyNeedsMigration(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: "1", State: client.STARTED, Node: "B"},
		client.Shard{Index: "index-1", Shard: "2", State: client.STARTED, Node: "C"},
	}
	assert.True(t, nodeIsMigratingData("A", shards), "no copy somewhere else")
}

func TestRelocationIsMigration(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: "0", State: client.RELOCATING, Node: "A"},
		client.Shard{Index: "index-1", Shard: "0", State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards), "await shard copy being relocated")
}

func TestCopyIsInitializing(t *testing.T) {
	shards := []client.Shard{
		client.Shard{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
		client.Shard{Index: "index-1", Shard: "0", State: client.INITIALIZING, Node: "B"},
	}
	assert.True(t, nodeIsMigratingData("A", shards), "shard copy exists but is still initializing")
}
