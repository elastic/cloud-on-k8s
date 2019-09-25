// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
)

// AllocationSetter captures Elasticsearch API calls around allocation filtering.
type AllocationSetter interface {
	ExcludeFromShardAllocation(nodes string) error
}

// ShardLister captures Elasticsearch API calls around shards retrieval.
type ShardLister interface {
	GetShards() (Shards, error)
}

type clientWrapper struct {
	client Client
}

func NewAllocationSetter(client Client) AllocationSetter {
	return &clientWrapper{client: client}
}

func (a *clientWrapper) ExcludeFromShardAllocation(nodes string) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()
	return a.client.ExcludeFromShardAllocation(ctx, nodes)
}

func NewShardLister(client Client) ShardLister {
	return &clientWrapper{client: client}
}

func (a *clientWrapper) GetShards() (Shards, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()
	return a.client.GetShards(ctx)
}
