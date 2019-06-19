// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cluster

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/discovery"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	espod "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	esreconcile "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	esversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("cluster")
)

var minVersion7 = version.MustParse("7.0.0")

type DirectCluster struct {
	esClient      client.Client
	observerState observer.State
}

func NewDirectCluster(esClient client.Client, state observer.State) *DirectCluster {
	return &DirectCluster{
		esClient:      esClient,
		observerState: state,
	}
}

// PrepareForDeletionPods informs Elasticsearch that it should prepare zero or more pods for permanent deletion.
//
// This ensures that the cluster is attempting to migrate data away from the nodes.
func (c *DirectCluster) PrepareForDeletionPods(
	k k8s.Client,
	es v1alpha1.Elasticsearch,
	podsState mutation.PodsState,
	performableChanges *mutation.PerformableChanges,
) error {
	if len(performableChanges.ToDelete) == 0 {
		return nil
	}

	toDeletePods := performableChanges.ToDelete.Pods()
	toDeleteNodeNames := espod.PodListToNames(toDeletePods)

	if err := migration.MigrateData(c.esClient, toDeleteNodeNames); err != nil {
		return err
	}

	return nil
}

// FilterDeletablePods ensures that only pods that are safely deletable from a data perspective are in the
// PerformableChanges.
func (c *DirectCluster) FilterDeletablePods(
	k k8s.Client,
	es v1alpha1.Elasticsearch,
	podsState mutation.PodsState,
	performableChanges *mutation.PerformableChanges,
) error {
	if len(performableChanges.ToDelete) == 0 {
		return nil
	}

	podsToDeleteWithoutCriticalData := c.podsWithoutCriticalData(performableChanges.ToDelete)

	minVersion, err := esversion.MinVersion(podsState.AllPods())
	if err != nil {
		return err
	}

	if !minVersion.IsSameOrAfter(minVersion7) {
		// 6.x nodes in cluster, assuming zen1

		// only delete a single master at a time, could be optimized to allow for deletes until quorum size changes
		masterPodFound := false

		var deletePods espod.PodsWithConfig
		for _, pod := range podsToDeleteWithoutCriticalData {
			if label.IsMasterNode(pod.Pod) {
				if masterPodFound {
					continue
				}
				masterPodFound = true
			}
			deletePods = append(deletePods, pod)
		}

		// TODO: zen1: verify that removing that master pod will not cause min master nodes invariant to be broken?
		performableChanges.ToDelete = deletePods
		return nil
	}

	// TODO: zen2: verify master-eligible pods to delete are actually in voting exclusions (cluster state internal?)
	performableChanges.ToDelete = podsToDeleteWithoutCriticalData
	return nil
}

// OnInfrastructureState prepares Elasticsearch for the changes we're about to do to the cluster.
func (c *DirectCluster) OnInfrastructureState(
	k k8s.Client,
	es v1alpha1.Elasticsearch,
	podsState mutation.PodsState,
	performableChanges *mutation.PerformableChanges,
	reconcileState *esreconcile.State,
) error {
	// no existing pods, nothing to do.
	if len(podsState.AllPods()) == 0 {
		return nil
	}

	minVersion, err := esversion.MinVersion(podsState.AllPods())
	if err != nil {
		return err
	}

	if !minVersion.IsSameOrAfter(minVersion7) {
		// 6.x nodes in cluster, assuming zen1:

		if len(podsState.MasterEligiblePods()) == 1 && len(label.MasterEligiblePods(performableChanges.ToCreate.Pods())) == 1 {
			// special case: going from 1 -> 2 masters, we cannot update the API until the new node has been created.
			return nil
		}

		// push new zen1 settings to the cluster through the cluster settings api
		if err := discovery.Zen1UpdateMinimumMasterNodesClusterSettings(
			c.esClient, podsState, performableChanges, reconcileState,
		); err != nil {
			return err
		}

		return nil
	}

	// zen2: exclude masters to delete without critical data from voting.
	podsToDeleteWithoutCriticalData := c.podsWithoutCriticalData(performableChanges.ToDelete)

	if err := discovery.Zen2SetVotingExclusions(c.esClient, podsToDeleteWithoutCriticalData.Pods()); err != nil {
		return err
	}

	return nil
}

// podsWithoutCriticalData returns the pods that do not have critical data.
//
// A pod is said to have no critical data if it and the rest of the deleting pods do not have the only copy of any
// shard.
func (c *DirectCluster) podsWithoutCriticalData(pods espod.PodsWithConfig) espod.PodsWithConfig {
	var withoutCriticalData espod.PodsWithConfig

	allDeletingPods := pods.Pods()

	for _, pod := range pods {
		// TODO: this `IsMigratingData` method could need a better name?
		if !migration.IsMigratingData(c.observerState, pod.Pod, allDeletingPods) {
			withoutCriticalData = append(withoutCriticalData, pod)
		}
	}

	return withoutCriticalData
}
