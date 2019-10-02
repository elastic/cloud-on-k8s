// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
)

// AllocationSetter captures Elasticsearch API calls around allocation filtering.
type AllocationSetter interface {
	// ExcludeFromShardAllocation takes a comma-separated string of node names and
	// configures transient allocation exclusions for the given nodes.
	ExcludeFromShardAllocation(nodes string) error
}

// ShardLister captures Elasticsearch API calls around shards retrieval.
type ShardLister interface {
	GetShards() (Shards, error)
}

func (c *clientV6) ExcludeFromShardAllocation(nodes string) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()
	allocationSettings := ClusterRoutingAllocation{
		Transient: AllocationSettings{
			Cluster: ClusterRoutingSettings{
				Routing: RoutingSettings{
					Allocation: RoutingAllocationSettings{
						Exclude: AllocationExclude{
							Name: nodes,
						},
					},
				},
			},
		},
	}
	return c.put(ctx, "/_cluster/settings", allocationSettings, nil)
}

func (c *clientV6) GetShards() (Shards, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()
	var shards Shards
	if err := c.get(ctx, "/_cat/shards?format=json", &shards); err != nil {
		return shards, err
	}
	// Fix the name of the node
	shards.fixNodeNames()
	return shards, nil
}
