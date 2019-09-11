// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

// downscaleContext holds the context of this downscale, including clients and states,
// propagated from the main driver.
type downscaleContext struct {
	// clients
	k8sClient k8s.Client
	esClient  esclient.Client
	// driver states
	resourcesState reconcile.ResourcesState
	observedState  observer.State
	reconcileState *reconcile.State
	expectations   *expectations.Expectations
	// ES cluster
	es v1alpha1.Elasticsearch
}

// ssetDownscale helps with the downscale of a single StatefulSet.
// A StatefulSet removal (going from 0 to 0 replicas) is also considered as a Downscale here.
type ssetDownscale struct {
	statefulSet     appsv1.StatefulSet
	initialReplicas int32
	targetReplicas  int32
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

// isRemoval returns true if this downscale is a StatefulSet removal.
func (d ssetDownscale) isRemoval() bool {
	// StatefulSet does not have any replica, and should not have one
	return d.initialReplicas == 0 && d.targetReplicas == 0
}

// isReplicaDecrease returns true if this downscale corresponds to decreasing replicas.
func (d ssetDownscale) isReplicaDecrease() bool {
	return d.targetReplicas < d.initialReplicas
}

// leavingNodeNames returns the names of all nodes that should leave the cluster (across StatefulSets).
func leavingNodeNames(downscales []ssetDownscale) []string {
	leavingNodes := []string{}
	for _, d := range downscales {
		leavingNodes = append(leavingNodes, d.leavingNodeNames()...)
	}
	return leavingNodes
}
