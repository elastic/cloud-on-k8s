// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"context"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var log = logf.Log.WithName("migrate-data")

const (
	// AllocationExcludeAnnotationName is the name of the annotation that stores the last
	// cluster.routing.allocation._name setting applied to the Elasticsearch cluster.
	AllocationExcludeAnnotationName = "elasticsearch.k8s.elastic.co/allocation-exclude"
)

func shardIsMigrating(toMigrate esclient.Shard, others []esclient.Shard) bool {
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
func nodeIsMigratingData(nodeName string, shards esclient.Shards, exclusions map[string]struct{}) bool {
	// all other shards not living on the node that is about to go away mapped to their corresponding shard keys
	othersByShard := make(map[string][]esclient.Shard)
	// all shard copies currently living on the node leaving the cluster
	candidates := make([]esclient.Shard, 0)

	// divide all shards into the to groups: migration candidate or other shard copy
	for _, shard := range shards {
		_, ignore := exclusions[shard.NodeName]
		if shard.NodeName == nodeName {
			candidates = append(candidates, shard)
		} else if !ignore {
			key := shard.Key()
			others, found := othersByShard[key]
			if !found {
				othersByShard[key] = []esclient.Shard{shard}
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
func IsMigratingData(ctx context.Context, shardLister esclient.ShardLister, podName string, exclusions []string) (bool, error) {
	shards, err := shardLister.GetShards(ctx)
	if err != nil {
		return false, err
	}
	excludedNodes := make(map[string]struct{}, len(exclusions))
	for _, name := range exclusions {
		excludedNodes[name] = struct{}{}
	}
	return nodeIsMigratingData(podName, shards, excludedNodes), nil
}

// allocationExcludeFromAnnotation returns the allocation exclude value stored in an annotation.
// May be empty if not set.
func allocationExcludeFromAnnotation(es esv1.Elasticsearch) string {
	return es.Annotations[AllocationExcludeAnnotationName]
}

// updateAllocationExcludeAnnotation sets an annotation in ES with the given cluster routing allocation exclude value.
// This is to avoid making the same ES API call over and over again.
func updateAllocationExcludeAnnotation(c k8s.Client, es esv1.Elasticsearch, value string) error {
	if es.Annotations == nil {
		es.Annotations = map[string]string{}
	}
	es.Annotations[AllocationExcludeAnnotationName] = value
	return c.Update(&es)
}

// MigrateData sets allocation filters for the given nodes.
func MigrateData(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	allocationSetter esclient.AllocationSetter,
	leavingNodes []string,
) error {
	// compute the expected exclusion value
	exclusions := "none_excluded"
	if len(leavingNodes) > 0 {
		exclusions = strings.Join(leavingNodes, ",")
	}
	// compare with what was set previously
	// Note the user may have changed it behind our back through the ES API. It is considered their responsibility.
	// Manually removing the annotation to force a refresh of the allocations exclude setting is a valid use case.
	if exclusions == allocationExcludeFromAnnotation(es) {
		return nil
	}
	log.Info("Setting routing allocation excludes", "namespace", es.Namespace, "es_name", es.Name, "value", exclusions)
	if err := allocationSetter.ExcludeFromShardAllocation(ctx, exclusions); err != nil {
		return err
	}
	// store updated value in an annotation so we don't make the same call over and over again
	return updateAllocationExcludeAnnotation(c, es, exclusions)
}
