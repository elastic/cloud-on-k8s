// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

func (d *defaultDriver) HandleDownscale(
	expectedStatefulSets sset.StatefulSetList,
	actualStatefulSets sset.StatefulSetList,
	esClient esclient.Client,
	observedState observer.State,
	reconcileState *reconcile.State,
) *reconciler.Results {
	results := &reconciler.Results{}

	// compute the list of nodes leaving the cluster, from which
	// data should be migrated away
	leavingNodes := []string{}

	// process each statefulset for downscale
	for i, actual := range actualStatefulSets {
		expected, shouldExist := expectedStatefulSets.GetByName(actual.Name)
		targetReplicas := int32(0) // sset removal
		if shouldExist {           // sset downscale
			targetReplicas = sset.Replicas(expected)
		}
		leaving, removalResult := d.scaleStatefulSetDown(actualStatefulSets, &actualStatefulSets[i], targetReplicas, esClient, observedState, reconcileState)
		results.WithResults(removalResult)
		if removalResult.HasError() {
			return results
		}
		leavingNodes = append(leavingNodes, leaving...)
	}

	// migrate data away from nodes leaving the cluster
	log.V(1).Info("Migrating data away from nodes", "nodes", leavingNodes)
	if err := migration.MigrateData(esClient, leavingNodes); err != nil {
		return results.WithError(err)
	}

	return results
}

// scaleStatefulSetDown scales the given StatefulSet down to targetReplicas, if possible.
// It returns the names of the nodes that will leave the cluster.
func (d *defaultDriver) scaleStatefulSetDown(
	allStatefulSets sset.StatefulSetList,
	ssetToScaleDown *appsv1.StatefulSet,
	targetReplicas int32,
	esClient esclient.Client,
	observedState observer.State,
	reconcileState *reconcile.State,
) ([]string, *reconciler.Results) {
	results := &reconciler.Results{}
	logger := log.WithValues("statefulset", k8s.ExtractNamespacedName(ssetToScaleDown))

	if sset.Replicas(*ssetToScaleDown) == 0 && targetReplicas == 0 {
		// no replicas expected, StatefulSet can be safely deleted
		logger.Info("Deleting statefulset", "namespace", ssetToScaleDown.Namespace, "name", ssetToScaleDown.Name)
		if err := d.Client.Delete(ssetToScaleDown); err != nil {
			return nil, results.WithError(err)
		}
	}
	// copy the current replicas, to be decremented with nodes to remove
	initialReplicas := sset.Replicas(*ssetToScaleDown)
	updatedReplicas := initialReplicas

	// leaving nodes names can be built from StatefulSet name and ordinals
	// nodes are ordered by highest ordinal first
	var leavingNodes []string
	for i := initialReplicas - 1; i > targetReplicas-1; i-- {
		leavingNodes = append(leavingNodes, sset.PodName(ssetToScaleDown.Name, i))
	}

	// TODO: don't remove last master/last data nodes?
	// TODO: detect cases where data migration cannot happen since no nodes to host shards?

	for _, node := range leavingNodes {
		if migration.IsMigratingData(observedState, node, leavingNodes) {
			// data migration not over yet: schedule a requeue
			logger.V(1).Info("Data migration not over yet, skipping node deletion", "node", node)
			results.WithResult(defaultRequeue)
			// no need to check other nodes since we remove them in order and this one isn't ready anyway
			break
		}
		// data migration over: allow pod to be removed
		updatedReplicas--
	}

	if updatedReplicas < initialReplicas {
		// trigger deletion of nodes whose data migration is over
		logger.V(1).Info("Scaling replicas down", "from", initialReplicas, "to", updatedReplicas)
		ssetToScaleDown.Spec.Replicas = &updatedReplicas

		if label.IsMasterNodeSet(*ssetToScaleDown) {
			// Update Zen1 minimum master nodes API, accounting for the updated downscaled replicas.
			_, err := zen1.UpdateMinimumMasterNodes(d.Client, d.ES, esClient, allStatefulSets, reconcileState)
			if err != nil {
				return nil, results.WithError(err)
			}
			// Update zen2 settings to exclude leaving master nodes from voting.
			excludeNodes := make([]string, 0, initialReplicas-updatedReplicas)
			for i := updatedReplicas; i < initialReplicas; i++ {
				excludeNodes = append(excludeNodes, sset.PodName(ssetToScaleDown.Name, i))
			}
			if err := zen2.AddToVotingConfigExclusions(esClient, *ssetToScaleDown, excludeNodes); err != nil {
				return nil, results.WithError(err)
			}
		}

		if err := d.Client.Update(ssetToScaleDown); err != nil {
			return nil, results.WithError(err)
		}
		// Expect the updated statefulset in the cache for next reconciliation.
		d.Expectations.ExpectGeneration(ssetToScaleDown.ObjectMeta)
	}

	return leavingNodes, results
}
