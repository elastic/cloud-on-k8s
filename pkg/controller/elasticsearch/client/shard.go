// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
)

// AllocationSetter captures Elasticsearch API calls around allocation filtering.
type AllocationSetter interface {
	// ExcludeFromShardAllocation takes a comma-separated string of node names and
	// configures transient allocation exclusions for the given nodes.
	ExcludeFromShardAllocation(ctx context.Context, nodes string) error
}

// ShardLister captures Elasticsearch API calls around shards retrieval.
type ShardLister interface {
	HasShardActivity(ctx context.Context) (bool, error)
	GetShards(ctx context.Context) (Shards, error)
}

func (c *clientV6) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
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

func (c *clientV6) GetShards(ctx context.Context) (Shards, error) {
	var shards Shards
	if err := c.get(ctx, "/_cat/shards?format=json", &shards); err != nil {
		return shards, err
	}
	return shards, nil
}

func (c *clientV6) HasShardActivity(ctx context.Context) (bool, error) {
	health, err := c.GetClusterHealth(ctx)
	if err != nil {
		return false, err
	}
	return health.HasShardActivity(), nil
}
