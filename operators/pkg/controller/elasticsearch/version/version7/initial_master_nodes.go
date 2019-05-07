// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
)

// ClusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set if the cluster is bootstrapping.
func ClusterInitialMasterNodesEnforcer(
	performableChanges mutation.PerformableChanges,
	resourcesState reconcile.ResourcesState,
) (*mutation.PerformableChanges, error) {
	var masterEligibleNodeNames []string
	for _, pod := range resourcesState.CurrentPods {
		if label.IsMasterNode(pod.Pod) {
			masterEligibleNodeNames = append(masterEligibleNodeNames, pod.Pod.Name)
		}
	}

	// if we have masters in the cluster, we can relatively safely assume that it's already bootstrapped
	if len(masterEligibleNodeNames) > 0 {
		return &performableChanges, nil
	}

	// collect the master eligible node names from the pods we're about to create
	for _, change := range performableChanges.ToCreate {
		if label.IsMasterNode(change.Pod) {
			masterEligibleNodeNames = append(masterEligibleNodeNames, change.Pod.Name)
		}
	}

	// make every master node in the cluster aware of the others:
	for i, change := range performableChanges.ToCreate {
		if !label.IsMasterNode(change.Pod) {
			// we only need to set this on master nodes
			continue
		}

		err := performableChanges.ToCreate[i].PodSpecCtx.Config.SetStrings(
			settings.ClusterInitialMasterNodes,
			masterEligibleNodeNames...,
		)
		if err != nil {
			return nil, err
		}
	}

	return &performableChanges, nil
}
