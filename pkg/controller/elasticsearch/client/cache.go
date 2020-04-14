// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"reflect"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("cached-client")

type CachedClientBuilder interface {

	// NewElasticsearchCachedClient returns an Elasticsearch client that relies on a cache for some operations.
	// Important: do not use the client concurrently for a same operation. Concurrent calls of a given operation is not thread-safe.
	NewElasticsearchCachedClient(
		es types.NamespacedName,
		client Client,
	) Client

	Forget(es types.NamespacedName)
}

// NewCachedClientBuilder returns a builder to create cached clients.
func NewCachedClientBuilder() CachedClientBuilder {
	return &cache{states: map[types.NamespacedName]*cachedState{}}
}

type cachedState struct {
	es                          types.NamespacedName
	allocationSettings          *string
	zen1MinimumMasterNodes      *int
	votingConfigExclusionsState *votingConfigExclusionsState
}

type votingConfigExclusionsState struct {
	initialized            bool
	votingConfigExclusions []string
}

var _ CachedClientBuilder = &cache{}

type cache struct {
	mu     sync.RWMutex
	states map[types.NamespacedName]*cachedState
}

var _ Client = &cachedClient{}

type cachedClient struct {
	Client
	*cachedState
}

func (c *cache) getState(es types.NamespacedName) (value *cachedState, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok = c.states[es]
	return
}

func (c *cache) newState(es types.NamespacedName) *cachedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if value, ok := c.states[es]; ok {
		return value
	}
	c.states[es] = &cachedState{
		es:                          es,
		votingConfigExclusionsState: &votingConfigExclusionsState{},
	}
	return c.states[es]
}

func (c *cache) Forget(es types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, es)
}

func (c *cache) NewElasticsearchCachedClient(
	es types.NamespacedName,
	client Client,
) Client {
	cachedState, ok := c.getState(es)
	if !ok {
		cachedState = c.newState(es)
	}
	return &cachedClient{
		Client:      client,
		cachedState: cachedState,
	}
}

func (c *cachedClient) Equal(c2 Client) bool {
	other, ok := c2.(*cachedClient)
	if !ok {
		return false
	}
	return c.Client.Equal(other.Client)
}

func (c *cachedClient) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	// Sort the node names in order to have a stable comparison
	sort.Strings(nodeNames)
	if c.votingConfigExclusionsState.initialized && reflect.DeepEqual(nodeNames, c.votingConfigExclusionsState.votingConfigExclusions) {
		log.V(1).Info("Cached voting config exclusions", "namespace", c.es.Namespace, "es_name", c.es.Name, "nodes", nodeNames)
		return nil
	}
	log.Info("Setting voting config exclusions", "namespace", c.es.Namespace, "nodes", nodeNames)
	if err := c.Client.AddVotingConfigExclusions(ctx, nodeNames, timeout); err != nil {
		c.votingConfigExclusionsState.initialized = false
		c.votingConfigExclusionsState.votingConfigExclusions = nil
		return err
	}
	c.votingConfigExclusionsState.initialized = true
	c.votingConfigExclusionsState.votingConfigExclusions = nodeNames
	return nil
}

func (c *cachedClient) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	if c.votingConfigExclusionsState.initialized && len(c.votingConfigExclusionsState.votingConfigExclusions) == 0 {
		log.V(1).Info("Cached delete voting config exclusions", "namespace", c.es.Namespace, "es_name", c.es.Name)
		return nil
	}
	log.Info("Delete voting config exclusions", "namespace", c.es.Namespace, "es_name", c.es.Name)
	if err := c.Client.DeleteVotingConfigExclusions(ctx, waitForRemoval); err != nil {
		c.votingConfigExclusionsState.initialized = false
		c.votingConfigExclusionsState.votingConfigExclusions = nil
		return err
	}
	c.votingConfigExclusionsState.initialized = true
	c.votingConfigExclusionsState.votingConfigExclusions = nil
	return nil
}

func (c *cachedClient) SetMinimumMasterNodes(ctx context.Context, minimumMasterNodes int) error {
	if c.zen1MinimumMasterNodes != nil && *c.zen1MinimumMasterNodes == minimumMasterNodes {
		log.V(1).Info("Cached minimum master nodes",
			"how", "api",
			"namespace", c.es.Namespace,
			"es_name", c.es.Name,
			"minimum_master_nodes", minimumMasterNodes,
		)
		return nil
	}
	log.Info("Updating minimum master nodes",
		"how", "api",
		"namespace", c.es.Namespace,
		"es_name", c.es.Name,
		"minimum_master_nodes", minimumMasterNodes,
	)
	if err := c.Client.SetMinimumMasterNodes(ctx, minimumMasterNodes); err != nil {
		c.zen1MinimumMasterNodes = nil
		return err
	}
	// Update cache
	c.zen1MinimumMasterNodes = &minimumMasterNodes
	return nil
}

func (c *cachedClient) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
	if c.allocationSettings != nil && *c.allocationSettings == nodes {
		log.V(1).Info("Cached routing allocation excludes", "namespace", c.es.Namespace, "es_name", c.es.Name, "value", nodes)
		return nil
	}
	log.Info("Setting routing allocation excludes", "namespace", c.es.Namespace, "es_name", c.es.Name, "value", nodes)
	if err := c.Client.ExcludeFromShardAllocation(ctx, nodes); err != nil {
		c.allocationSettings = nil
		return err
	}
	// Update cache
	c.allocationSettings = &nodes
	return nil
}
