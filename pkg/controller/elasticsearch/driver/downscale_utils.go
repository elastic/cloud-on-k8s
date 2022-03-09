// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// downscaleContext holds the context of this downscale, including clients and states,
// propagated from the main driver.
type downscaleContext struct {
	// clients
	k8sClient    k8s.Client
	esClient     esclient.Client
	nodeShutdown shutdown.Interface
	// driver states
	resourcesState reconcile.ResourcesState
	reconcileState *reconcile.State
	expectations   *expectations.Expectations
	// ES cluster
	es esv1.Elasticsearch

	parentCtx context.Context
}

func newDownscaleContext(
	ctx context.Context,
	k8sClient k8s.Client,
	esClient esclient.Client,
	resourcesState reconcile.ResourcesState,
	reconcileState *reconcile.State,
	expectations *expectations.Expectations,
	// ES cluster
	es esv1.Elasticsearch,
	nodeShutdown shutdown.Interface,
) downscaleContext {
	return downscaleContext{
		k8sClient:      k8sClient,
		esClient:       esClient,
		nodeShutdown:   nodeShutdown,
		resourcesState: resourcesState,
		reconcileState: reconcileState,
		es:             es,
		expectations:   expectations,
		parentCtx:      ctx,
	}
}

// ssetDownscale helps with the downscale of a single StatefulSet.
type ssetDownscale struct {
	statefulSet     appsv1.StatefulSet
	initialReplicas int32
	targetReplicas  int32
	finalReplicas   int32
}

// leavingNodeNames returns names of the nodes that are supposed to leave the Elasticsearch cluster
// for this StatefulSet. They are ordered by highest ordinal first;
func (d ssetDownscale) leavingNodeNames() []string {
	if d.targetReplicas >= d.initialReplicas {
		return nil
	}
	leavingNodes := make([]string, 0, d.initialReplicas-d.targetReplicas)
	for i := d.initialReplicas - 1; i >= d.targetReplicas; i-- {
		leavingNodes = append(leavingNodes, sset.PodName(d.statefulSet.Name, i))
	}
	return leavingNodes
}

// leavingNodeNames returns the names of all nodes that should leave the cluster (across StatefulSets).
func leavingNodeNames(downscales []ssetDownscale) []string {
	leavingNodes := []string{}
	for _, d := range downscales {
		leavingNodes = append(leavingNodes, d.leavingNodeNames()...)
	}
	return leavingNodes
}
