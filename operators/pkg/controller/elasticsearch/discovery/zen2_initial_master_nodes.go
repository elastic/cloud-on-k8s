// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package discovery

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
)

// Zen2InjectInitialMasterNodesIfBootstrapping enforces that cluster.initial_master_nodes is set for the master nodes
// that we're about to create if they are the first nodes to be created in the cluster.
func Zen2InjectInitialMasterNodesIfBootstrapping(
	performableChanges *mutation.PerformableChanges,
	resourcesState reconcile.ResourcesState,
) error {
	if len(resourcesState.CurrentPods) != 0 {
		// already have pods, no bootstrap should be done
		// this means if we lose all master nodes, but still have other nodes, we will not automatically bootstrap
		return nil
	}

	if len(performableChanges.ToCreate) == 0 {
		// not creating any nodes, no bootstrapping can be done
		return nil
	}

	minVersion, err := version.MinVersion(performableChanges.ToCreate.Pods())
	if err != nil {
		return err
	}

	if !minVersion.IsSameOrAfter(Zen2MinimumVersion) {
		// not creating zen2 pods, no zen 2 bootstrapping
		return nil
	}

	var masterEligibleNodeNames []string
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

		if err := performableChanges.ToCreate[i].PodSpecCtx.Config.SetStrings(
			settings.ClusterInitialMasterNodes,
			masterEligibleNodeNames...,
		); err != nil {
			return err
		}
	}

	return nil
}
