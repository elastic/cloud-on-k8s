// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"sync"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// ESState gives information about Elasticsearch current status.
type ESState interface {
	// NodesInCluster returns true if the given nodes exist in the Elasticsearch cluster.
	NodesInCluster(nodeNames []string) (bool, error)
	// ShardAllocationsEnabled returns true if shards allocation are enabled in the cluster.
	ShardAllocationsEnabled() (bool, error)
	// Health returns the health of the Elasticsearch cluster.
	Health() (esclient.Health, error)
}

// MemoizingESState requests Elasticsearch for the requested information only once, at first call.
// It is "lazy" in the sense it only calls Elasticsearch if required, and does not pre-populate the state.
type MemoizingESState struct {
	esClient esclient.Client
	*memoizingNodes
	*memoizingShardsAllocationEnabled
	*memoizingHealth
}

// NewMemoizingESState returns an initialized MemoizingESState.
func NewMemoizingESState(ctx context.Context, esClient esclient.Client) ESState {
	return &MemoizingESState{
		esClient:                         esClient,
		memoizingNodes:                   &memoizingNodes{esClient: esClient, ctx: ctx},
		memoizingShardsAllocationEnabled: &memoizingShardsAllocationEnabled{esClient: esClient, ctx: ctx},
		memoizingHealth:                  &memoizingHealth{esClient: esClient, ctx: ctx},
	}
}

// initOnce calls f(), if not already called for the given once.
func initOnce(once *sync.Once, f func() error) error {
	var err error
	once.Do(func() {
		err = f()
	})
	return err
}

// -- Nodes

// memoizingNodes provides nodes information.
type memoizingNodes struct {
	once     sync.Once
	esClient esclient.Client
	ctx      context.Context
	nodes    []string
}

// initialize requests Elasticsearch for nodes information, only once.
func (n *memoizingNodes) initialize() error {
	ctx, cancel := context.WithTimeout(n.ctx, esclient.DefaultReqTimeout)
	defer cancel()
	nodes, err := n.esClient.GetNodes(ctx)
	if err != nil {
		return err
	}
	n.nodes = nodes.Names()
	return nil
}

// NodesInCluster returns true if the given nodes exist in the Elasticsearch cluster.
func (n *memoizingNodes) NodesInCluster(nodeNames []string) (bool, error) {
	if err := initOnce(&n.once, n.initialize); err != nil {
		return false, err
	}
	return stringsutil.StringsInSlice(nodeNames, n.nodes), nil
}

// -- Shards allocation enabled

// memoizingShardsAllocationEnabled provides shards allocation information.
type memoizingShardsAllocationEnabled struct {
	enabled  bool
	once     sync.Once
	esClient esclient.Client
	ctx      context.Context
}

// initialize requests Elasticsearch for shards allocation information, only once.
func (s *memoizingShardsAllocationEnabled) initialize() error {
	ctx, cancel := context.WithTimeout(s.ctx, esclient.DefaultReqTimeout)
	defer cancel()
	allocationSettings, err := s.esClient.GetClusterRoutingAllocation(ctx)
	if err != nil {
		return err
	}
	s.enabled = allocationSettings.Transient.IsShardsAllocationEnabled()
	return nil
}

// ShardAllocationsEnabled returns true if shards allocation are enabled in the cluster.
func (s *memoizingShardsAllocationEnabled) ShardAllocationsEnabled() (bool, error) {
	if err := initOnce(&s.once, s.initialize); err != nil {
		return false, err
	}
	return s.enabled, nil
}

// -- Cluster Status

// memoizingHealth provides cluster health information.
type memoizingHealth struct {
	health   esclient.Health
	once     sync.Once
	esClient esclient.Client
	ctx      context.Context
}

// initialize requests Elasticsearch for cluster health, only once.
func (h *memoizingHealth) initialize() error {
	ctx, cancel := context.WithTimeout(h.ctx, esclient.DefaultReqTimeout)
	defer cancel()

	// get cluster health but make sure we have no pending shard initializations
	// by requiring the event queue to be empty
	health, err := h.esClient.GetClusterHealthWaitForAllEvents(ctx)
	if err != nil {
		return err
	}
	h.health = health
	return nil
}

// Health returns the cluster health.
func (h *memoizingHealth) Health() (esclient.Health, error) {
	if err := initOnce(&h.once, h.initialize); err != nil {
		return esclient.Health{}, err
	}
	return h.health, nil
}
