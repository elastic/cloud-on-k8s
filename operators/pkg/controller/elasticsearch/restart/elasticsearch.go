// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"context"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
)

// PrepareClusterForNodesStop performs cluster-wide ES requests to speedup the restart process.
// See https://www.elastic.co/guide/en/elasticsearch/reference/6.7/restart-upgrade.html.
func PrepareClusterForNodesStop(esClient client.Client) error {
	// disable shard allocation to ensure shards from stopped nodes
	// won't be moved around during the restart process
	log.V(1).Info("Disabling replica shards allocation")
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := esClient.DisableReplicasShardAllocation(ctx); err != nil {
		return err
	}

	// perform a synced flush (best effort) to speedup shard recovery
	// any ongoing indexing operation on a particular shard will make the sync flush
	// fail for that particular shard, that's ok.
	ctx2, cancel2 := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel2()
	if err := esClient.SyncedFlush(ctx2); err != nil {
		return err
	}

	return nil
}
