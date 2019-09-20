// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

func shardIsMigrating(toMigrate client.Shard, others []client.Shard) bool {
	if toMigrate.IsRelocating() || toMigrate.IsInitializing() {
		return true // being migrated away or weirdly just initializing
	}
	if !toMigrate.IsStarted() {
		return false // early return as we are interested only in started shards for migration purposes
	}
	for _, otherCopy := range others {
		if otherCopy.IsStarted() {
			return false // found another shard copy
		}
	}
	return true // we assume other copies are initializing or there are no other copies
}

// nodeIsMigratingData is the core of IsMigratingData just with any I/O
// removed to facilitate testing. See IsMigratingData for a high-level description.
func nodeIsMigratingData(nodeName string, shards client.Shards, exclusions map[string]struct{}) bool {
	// all other shards not living on the node that is about to go away mapped to their corresponding shard keys
	othersByShard := make(map[string][]client.Shard)
	// all shard copies currently living on the node leaving the cluster
	candidates := make([]client.Shard, 0)

	// divide all shards into the to groups: migration candidate or other shard copy
	for _, shard := range shards {
		_, ignore := exclusions[shard.Node]
		if shard.Node == nodeName {
			candidates = append(candidates, shard)
		} else if !ignore {
			key := shard.Key()
			others, found := othersByShard[key]
			if !found {
				othersByShard[key] = []client.Shard{shard}
			} else {
				othersByShard[key] = append(others, shard)
			}
		}
	}

	// check if there is at least one shard on this node that is migrating or needs to migrate
	for _, toMigrate := range candidates {
		if shardIsMigrating(toMigrate, othersByShard[toMigrate.Key()]) {
			return true
		}
	}
	return false

}

// IsMigratingData looks only at the presence of shards on a given node
// and checks if there is at least one other copy of the shard in the cluster
// that is started and not relocating.
func IsMigratingData(shardLister esclient.ShardLister, podName string, exclusions []string) (bool, error) {
	shards, err := shardLister.GetShards()
	if err != nil {
		return false, nil
	}
	excludedNodes := make(map[string]struct{}, len(exclusions))
	for _, name := range exclusions {
		excludedNodes[name] = struct{}{}
	}
	return nodeIsMigratingData(podName, shards, excludedNodes), nil
}

// MigrateData sets allocation filters for the given nodes.
func MigrateData(allocationSetter esclient.AllocationSetter, leavingNodes []string) error {
	exclusions := "none_excluded"
	if len(leavingNodes) > 0 {
		exclusions = strings.Join(leavingNodes, ",")
	}
	// update allocation exclusions
	return allocationSetter.ExcludeFromShardAllocation(exclusions)
}
