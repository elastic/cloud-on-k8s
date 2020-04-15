// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"context"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("migrate-data")

// NodeHasShard returns true if the given ES Pod is holding at least one shard (primary or replica).
func NodeHasShard(ctx context.Context, shardLister esclient.ShardLister, podName string) (bool, error) {
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

// MigrateData sets allocation filters for the given nodes.
func MigrateData(
	ctx context.Context,
	es esv1.Elasticsearch,
	allocationSetter esclient.AllocationSetter,
	leavingNodes []string,
) error {
	// compute the expected exclusion value
	exclusions := "none_excluded"
	if len(leavingNodes) > 0 {
		exclusions = strings.Join(leavingNodes, ",")
	}
	log.Info("Setting routing allocation excludes", "namespace", es.Namespace, "es_name", es.Name, "value", exclusions)
	return allocationSetter.ExcludeFromShardAllocation(ctx, exclusions)
}
