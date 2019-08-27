// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"sync"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// ESState gives information about Elasticsearch current status.
type ESState interface {
	// NodesInCluster returns true if the given nodes exist in the Elasticsearch cluster.
	NodesInCluster(nodeNames []string) (bool, error)
	// ShardAllocationsEnabled returns true if shards allocation are enabled in the cluster.
	ShardAllocationsEnabled() (bool, error)
	// GreenHealth returns true if the cluster health is currently green.
	GreenHealth() (bool, error)
}

// MemoizingESState requests Elasticsearch for the requested information only once, at first call.
// It is "lazy" in the sense it only calls Elasticsearch if required, and does not pre-populate the state.
type MemoizingESState struct {
	esClient esclient.Client
	*memoizingNodes
	*memoizingShardsAllocationEnabled
	*memoizingGreenHealth
}

// NewMemoizingESState returns an initialized MemoizingESState.
func NewMemoizingESState(esClient esclient.Client) ESState {
	return &MemoizingESState{
		esClient:                         esClient,
		memoizingNodes:                   &memoizingNodes{esClient: esClient},
		memoizingShardsAllocationEnabled: &memoizingShardsAllocationEnabled{esClient: esClient},
		memoizingGreenHealth:             &memoizingGreenHealth{esClient: esClient},
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
	nodes    []string
}

// initialize requests Elasticsearch for nodes information, only once.
func (n *memoizingNodes) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
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
}

// initialize requests Elasticsearch for shards allocation information, only once.
func (s *memoizingShardsAllocationEnabled) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
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

// -- Green health

// memoizingGreenHealth provides cluster health information.
type memoizingGreenHealth struct {
	greenHealth bool
	once        sync.Once
	esClient    esclient.Client
}

// initialize requests Elasticsearch for cluster health, only once.
func (h *memoizingGreenHealth) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	health, err := h.esClient.GetClusterHealth(ctx)
	if err != nil {
		return err
	}
	h.greenHealth = health.Status == string(v1alpha1.ElasticsearchGreenHealth)
	return nil
}

// GreenHealth returns true if the cluster health is currently green.
func (h *memoizingGreenHealth) GreenHealth() (bool, error) {
	if err := initOnce(&h.once, h.initialize); err != nil {
		return false, err
	}
	return h.greenHealth, nil
}
