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

// IsMigratingData looks only at the presence of shards on a given node
// and checks if there is at least one other copy of the shard in the cluster
// that is started and not relocating.
func IsMigratingData(ctx context.Context, shardLister esclient.ShardLister, podName string) (bool, error) {
	shards, err := shardLister.GetShards(ctx)
	if err != nil {
		return false, err
	}
	// filter shards affected by node removal
	for _, shard := range shards {
		if shard.NodeName == podName {
			return true, nil
		}
	}
	return false, nil
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
