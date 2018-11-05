package elasticsearch

import (
	"strings"
	"time"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"k8s.io/api/core/v1"
)

func shardIsMigrating(toMigrate client.Shard, others []client.Shard) bool {
	if toMigrate.IsRelocating() || toMigrate.IsInitializing() {
		return true //being migrated away or weirdly just initializing
	}
	found := others != nil
	if toMigrate.IsStarted() {
		if !found {
			return true //not running anywhere else
		}
		for _, other := range others {
			if other.IsStarted() {
				return false // found another shard copy
			}
		}
		return true // we assume other copies are initializing

	}
	return false //we assume no migration is happening at this point
}

func nodeIsMigratingData(nodeName string, shards []client.Shard) bool {
	othersByShard := make(map[string][]client.Shard)
	candidates := make([]client.Shard, 0)

	for _, shard := range shards {

		if shard.Node == nodeName {
			candidates = append(candidates, shard)
		} else {
			key := shard.Key()
			others, found := othersByShard[key]
			if !found {
				othersByShard[key] = []client.Shard{shard}
			} else {
				othersByShard[key] = append(others, shard)
			}
		}
	}

	for _, toMigrate := range candidates {
		migrating := shardIsMigrating(toMigrate, othersByShard[toMigrate.Key()])
		if migrating {
			return true
		}
	}
	return false

}

// IsMigrating Data looks only at the presence of shards on a given node
// and checks if there is at least one other copy of the shard in the cluster
// that is started and not relocating.
func IsMigratingData(c *client.Client, pod v1.Pod) (bool, error) {
	shards, err := c.GetShards()
	if err != nil {
		return true, err
	}
	return nodeIsMigratingData(pod.Name, shards), nil
}

//MigrateData sets allocation filters for the given pod
func MigrateData(client *client.Client, leavingNodes []string) error {
	var exclusions string
	if len(leavingNodes) == 0 {
		exclusions = "none_excluded"
	} else {
		// See https://github.com/elastic/elasticsearch/issues/28316
		withBugfix := append(leavingNodes, time.Now().String())
		exclusions = strings.Join(withBugfix, ",")
	}
	//update allocation exclusions
	return client.ExcludeFromShardAllocation(exclusions)
}
