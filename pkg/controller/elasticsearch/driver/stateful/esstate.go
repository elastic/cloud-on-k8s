// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"

	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
)

// ESState gives information about Elasticsearch current status.
type ESState interface {
	// NodesInCluster returns true if the given nodes exist in the Elasticsearch cluster.
	NodesInCluster(nodeNames []string) (bool, error)
	// NodeNameToID returns a map of Elasticsearch node ID to node name.
	NodeNameToID() (map[string]string, error)
	// NodesResponsePartial returns true if some nodes of the cluster did not respond to the nodes request that
	// NodeNameToID relies on: they may be members of the cluster although they have no node ID.
	NodesResponsePartial() (bool, error)
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

// initOnce calls f() only once, and returns its error to every caller, so that none reads an uninitialized state, for
// example no nodes, which would make every node look like it is not a member of the cluster.
type initOnce struct {
	once sync.Once
	err  error
}

// do calls f() if not already called, and returns its error.
func (i *initOnce) do(f func() error) error {
	i.once.Do(func() {
		i.err = f()
	})
	return i.err
}

// -- Nodes

// memoizingNodes provides nodes information.
type memoizingNodes struct {
	init         initOnce
	esClient     esclient.Client
	ctx          context.Context
	nodeNameToID map[string]string
	partial      bool
}

// initialize requests Elasticsearch for nodes information, only once.
func (n *memoizingNodes) initialize() error {
	nodes, err := n.esClient.GetNodes(n.ctx)
	if err != nil {
		return err
	}
	n.partial = nodes.IsPartial()
	n.nodeNameToID = nodeNamesToIDs(nodes)
	return nil
}

// NodesInCluster returns true if the given nodes exist in the Elasticsearch cluster.
func (n *memoizingNodes) NodesInCluster(nodeNames []string) (bool, error) {
	if err := n.init.do(n.initialize); err != nil {
		return false, err
	}
	for _, name := range nodeNames {
		if _, exists := n.nodeNameToID[name]; !exists {
			return false, nil
		}
	}
	return true, nil
}

func (n *memoizingNodes) NodeNameToID() (map[string]string, error) {
	if err := n.init.do(n.initialize); err != nil {
		return nil, err
	}
	return n.nodeNameToID, nil
}

func (n *memoizingNodes) NodesResponsePartial() (bool, error) {
	if err := n.init.do(n.initialize); err != nil {
		return false, err
	}
	return n.partial, nil
}

// -- Shards allocation enabled

// memoizingShardsAllocationEnabled provides shards allocation information.
type memoizingShardsAllocationEnabled struct {
	enabled  bool
	init     initOnce
	esClient esclient.Client
	ctx      context.Context
}

// initialize requests Elasticsearch for shards allocation information, only once.
func (s *memoizingShardsAllocationEnabled) initialize() error {
	allocationSettings, err := s.esClient.GetClusterRoutingAllocation(s.ctx)
	if err != nil {
		return err
	}
	s.enabled = allocationSettings.Transient.IsShardsAllocationEnabled()
	return nil
}

// ShardAllocationsEnabled returns true if shards allocation are enabled in the cluster.
func (s *memoizingShardsAllocationEnabled) ShardAllocationsEnabled() (bool, error) {
	if err := s.init.do(s.initialize); err != nil {
		return false, err
	}
	return s.enabled, nil
}

// membersAmong returns the given nodes that are members of the cluster.
func membersAmong(esState ESState, nodes []string) (sets.Set[string], error) {
	if len(nodes) == 0 {
		// do not require Elasticsearch to be reachable when there is nothing to look up
		return sets.New[string](), nil
	}
	nodeNameToID, err := esState.NodeNameToID()
	if err != nil {
		return nil, err
	}
	members := sets.New[string]()
	for _, node := range nodes {
		if _, exists := nodeNameToID[node]; exists {
			members.Insert(node)
		}
	}
	return members, nil
}

// -- Cluster Status

// memoizingHealth provides cluster health information.
type memoizingHealth struct {
	health   esclient.Health
	init     initOnce
	esClient esclient.Client
	ctx      context.Context
}

// initialize requests Elasticsearch for cluster health, only once.
func (h *memoizingHealth) initialize() error {
	// get cluster health but make sure we have no pending shard initializations
	// by requiring the event queue to be empty
	health, err := h.esClient.GetClusterHealthWaitForAllEvents(h.ctx)
	if err != nil {
		return err
	}
	h.health = health
	return nil
}

// Health returns the cluster health.
func (h *memoizingHealth) Health() (esclient.Health, error) {
	if err := h.init.do(h.initialize); err != nil {
		return esclient.Health{}, err
	}
	return h.health, nil
}
