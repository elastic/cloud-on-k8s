// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/observer"
	corev1 "k8s.io/api/core/v1"
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
func nodeIsMigratingData(nodeName string, shards []client.Shard, exclusions map[string]struct{}) bool {
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
func IsMigratingData(state observer.State, pod corev1.Pod, exclusions []corev1.Pod) bool {
	clusterState := state.ClusterState
	if clusterState.IsEmpty() {
		return true // we don't know if the request timed out or the cluster has not formed yet
	}
	excludedNodes := make(map[string]struct{}, len(exclusions))
	for _, n := range exclusions {
		excludedNodes[n.Name] = struct{}{}
	}
	return nodeIsMigratingData(pod.Name, clusterState.GetShards(), excludedNodes)
}

// AllocationSettings captures Elasticsearch API calls around allocation filtering.
type AllocationSettings interface {
	ExcludeFromShardAllocation(context context.Context, nodes string) error
}

// setAllocationExcludes sets allocation filters for the given nodes.
func setAllocationExcludes(asClient AllocationSettings, leavingNodes []string, now time.Time) error {
	exclusions := "none_excluded"
	if len(leavingNodes) > 0 {
		// See https://github.com/elastic/elasticsearch/issues/28316
		withBugfix := append(leavingNodes, strconv.FormatInt(now.Unix(), 10))
		exclusions = strings.Join(withBugfix, ",")
	}
	// update allocation exclusions
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	return asClient.ExcludeFromShardAllocation(ctx, exclusions)
}

// MigrateData sets allocation filters for the given nodes.
func MigrateData(client AllocationSettings, leavingNodes []string) error {
	return setAllocationExcludes(client, leavingNodes, time.Now())
}
