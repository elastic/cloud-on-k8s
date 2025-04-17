// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"
	"sync"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

type pendingChangesPerCluster struct {
	pendingChangesPerCluster map[types.NamespacedName]*pendingChanges
	mu                       sync.RWMutex
}

func NewProvider(c k8s.Client) *Provider {
	return &Provider{
		c: c,
		pendingChangesPerCluster: pendingChangesPerCluster{
			pendingChangesPerCluster: make(map[types.NamespacedName]*pendingChanges),
		},
	}
}

type Provider struct {
	c                        k8s.Client
	pendingChangesPerCluster pendingChangesPerCluster
}

func (p *Provider) ForgetCluster(name types.NamespacedName) {
	if p == nil {
		return
	}
	p.pendingChangesPerCluster.mu.Lock()
	defer p.pendingChangesPerCluster.mu.Unlock()
	delete(p.pendingChangesPerCluster.pendingChangesPerCluster, name)
}

func (p *Provider) ForCluster(ctx context.Context, log logr.Logger, owner *esv1.Elasticsearch) (*APIKeyStore, error) {
	if p == nil {
		return nil, nil
	}
	name := types.NamespacedName{
		Namespace: owner.Namespace,
		Name:      owner.Name,
	}
	pendingChanges := p.forCluster(name)
	if pendingChanges != nil {
		return loadAPIKeyStore(ctx, log, p.c, owner, pendingChanges)
	}
	return loadAPIKeyStore(ctx, log, p.c, owner, p.newForCluster(name))
}

func (p *Provider) forCluster(name types.NamespacedName) *pendingChanges {
	if p == nil {
		return nil
	}
	p.pendingChangesPerCluster.mu.RLock()
	defer p.pendingChangesPerCluster.mu.RUnlock()
	return p.pendingChangesPerCluster.pendingChangesPerCluster[name]
}

func (p *Provider) newForCluster(name types.NamespacedName) *pendingChanges {
	if p == nil {
		return nil
	}
	p.pendingChangesPerCluster.mu.Lock()
	defer p.pendingChangesPerCluster.mu.Unlock()
	// Check if another goroutine did not create the pending changes
	currentPendingChanges := p.pendingChangesPerCluster.pendingChangesPerCluster[name]
	if currentPendingChanges != nil {
		return currentPendingChanges
	}
	newPendingChanges := &pendingChanges{
		changes: make(map[string]pendingChange),
	}
	p.pendingChangesPerCluster.pendingChangesPerCluster[name] = newPendingChanges
	return newPendingChanges
}
