// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

const (
	// ClusterUUIDAnnotationName used to store the cluster UUID as an annotation when cluster has been bootstrapped.
	ClusterUUIDAnnotationName = "elasticsearch.k8s.elastic.co/cluster-uuid"
)

// ClusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set if the cluster is bootstrapping.
// It's also save the cluster UUID as an annotation to ensure that it's not set if the cluster has already been bootstrapped.
func ClusterInitialMasterNodesEnforcer(
	cluster v1alpha1.Elasticsearch,
	clusterState observer.State,
	c k8s.Client,
	performableChanges mutation.PerformableChanges,
	resourcesState reconcile.ResourcesState,
) (*mutation.PerformableChanges, error) {

	// Check if the cluster has an UUID, if not try to fetch it from the observer state and store it as an annotation.
	_, ok := cluster.Annotations[ClusterUUIDAnnotationName]
	if ok {
		// existence of the annotation shows that the cluster has been bootstrapped
		return &performableChanges, nil
	}

	// no annotation, let see if the cluster has been bootstrapped by looking at it's UUID
	if clusterState.ClusterState != nil && len(clusterState.ClusterState.ClusterUUID) > 0 {
		// UUID is set, let's update the annotation on the Elasticsearch object
		if cluster.Annotations == nil {
			cluster.Annotations = make(map[string]string)
		}
		cluster.Annotations[ClusterUUIDAnnotationName] = clusterState.ClusterState.ClusterUUID
		if err := c.Update(&cluster); err != nil {
			return nil, err
		}
		return &performableChanges, nil
	}

	var masterEligibleNodeNames []string
	for _, pod := range resourcesState.CurrentPods {
		if label.IsMasterNode(pod.Pod) {
			masterEligibleNodeNames = append(masterEligibleNodeNames, pod.Pod.Name)
		}
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
