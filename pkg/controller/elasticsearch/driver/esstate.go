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

type ESState interface {
	NodesInCluster(nodeNames []string) (bool, error)
	ShardAllocationsEnabled() (bool, error)
	GreenHealth() (bool, error)
}

type LazyESState struct {
	esClient esclient.Client
	*lazyNodes
	*lazyShardsAllocationEnabled
	*lazyGreenHealth
}

func NewLazyESState(esClient esclient.Client) ESState {
	return &LazyESState{
		esClient:                    esClient,
		lazyNodes:                   &lazyNodes{esClient: esClient},
		lazyShardsAllocationEnabled: &lazyShardsAllocationEnabled{esClient: esClient},
		lazyGreenHealth:             &lazyGreenHealth{esClient: esClient},
	}
}

func initOnce(once *sync.Once, f func() error) error {
	var err error
	once.Do(func() {
		err = f()
	})
	return err
}

// -- Nodes

type lazyNodes struct {
	once     sync.Once
	esClient esclient.Client
	nodes    []string
}

func (n *lazyNodes) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	nodes, err := n.esClient.GetNodes(ctx)
	if err != nil {
		return err
	}
	n.nodes = nodes.Names()
	return nil
}

func (n *lazyNodes) NodesInCluster(nodeNames []string) (bool, error) {
	if err := initOnce(&n.once, n.initialize); err != nil {
		return false, err
	}
	return stringsutil.StringsInSlice(nodeNames, n.nodes), nil
}

// -- Shards allocation enabled

type lazyShardsAllocationEnabled struct {
	enabled  bool
	once     sync.Once
	esClient esclient.Client
}

func (s *lazyShardsAllocationEnabled) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	allocationSettings, err := s.esClient.GetClusterRoutingAllocation(ctx)
	if err != nil {
		return err
	}
	s.enabled = allocationSettings.Transient.IsShardsAllocationEnabled()
	return nil
}

func (s *lazyShardsAllocationEnabled) ShardAllocationsEnabled() (bool, error) {
	if err := initOnce(&s.once, s.initialize); err != nil {
		return false, err
	}
	return s.enabled, nil
}

// -- Green health

type lazyGreenHealth struct {
	greenHealth bool
	once        sync.Once
	esClient    esclient.Client
}

func (h *lazyGreenHealth) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	health, err := h.esClient.GetClusterHealth(ctx)
	if err != nil {
		return err
	}
	h.greenHealth = health.Status == string(v1alpha1.ElasticsearchGreenHealth)
	return nil
}

func (h *lazyGreenHealth) GreenHealth() (bool, error) {
	if err := initOnce(&h.once, h.initialize); err != nil {
		return false, err
	}
	return h.greenHealth, nil
}
