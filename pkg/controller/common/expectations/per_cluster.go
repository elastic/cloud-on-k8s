// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ClustersExpectation stores Expectations for several clusters.
// It is thread-safe, but the underlying per-cluster Expectations is not.
type ClustersExpectation struct {
	client   k8s.Client
	clusters map[types.NamespacedName]*Expectations
	lock     sync.RWMutex
}

// NewClustersExpectations returns an initialized ClustersExpectation.
func NewClustersExpectations(client k8s.Client) *ClustersExpectation {
	return &ClustersExpectation{
		client:   client,
		clusters: map[types.NamespacedName]*Expectations{},
		lock:     sync.RWMutex{},
	}
}

// ForCluster returns the expectations for the given cluster.
func (c *ClustersExpectation) ForCluster(cluster types.NamespacedName) *Expectations {
	expectations, ok := c.get(cluster)
	if !ok {
		expectations = c.create(cluster)
	}
	return expectations
}

// RemoveCluster removes existing expectations for the given cluster.
func (c *ClustersExpectation) RemoveCluster(cluster types.NamespacedName) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.clusters, cluster)
}

func (c *ClustersExpectation) get(cluster types.NamespacedName) (*Expectations, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	expectations, ok := c.clusters[cluster]
	return expectations, ok
}

func (c *ClustersExpectation) create(cluster types.NamespacedName) *Expectations {
	expectations := NewExpectations(c.client)
	c.lock.Lock()
	defer c.lock.Unlock()
	c.clusters[cluster] = expectations
	return expectations
}
