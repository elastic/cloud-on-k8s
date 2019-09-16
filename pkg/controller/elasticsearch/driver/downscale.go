// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
)

// HandleDownscale attempts to downscale actual StatefulSets towards expected ones.
func HandleDownscale(
	downscaleCtx downscaleContext,
	expectedStatefulSets sset.StatefulSetList,
	actualStatefulSets sset.StatefulSetList,
) *reconciler.Results {
	results := &reconciler.Results{}

	// compute the list of StatefulSet downscales to perform
	downscales := calculateDownscales(expectedStatefulSets, actualStatefulSets)
	leavingNodes := leavingNodeNames(downscales)

	// migrate data away from nodes that should be removed
	if err := scheduleDataMigrations(downscaleCtx.esClient, leavingNodes); err != nil {
		return results.WithError(err)
	}

	// make sure we only downscale nodes we're allowed to
	downscaleState, err := newDownscaleState(downscaleCtx.k8sClient, downscaleCtx.es)
	if err != nil {
		return results.WithError(err)
	}

	for _, downscale := range downscales {
		// attempt the StatefulSet downscale (may or may not remove nodes)
		requeue, err := attemptDownscale(downscaleCtx, downscale, downscaleState, leavingNodes, actualStatefulSets)
		if err != nil {
			return results.WithError(err)
		}
		if requeue {
			// retry downscaling this statefulset later
			results.WithResult(defaultRequeue)
		}
	}

	return results
}

// calculateDownscales compares expected and actual StatefulSets to return a list of ssetDownscale.
// We also include StatefulSets removal (0 replicas) in those downscales.
func calculateDownscales(expectedStatefulSets sset.StatefulSetList, actualStatefulSets sset.StatefulSetList) []ssetDownscale {
	downscales := []ssetDownscale{}
	for _, actualSset := range actualStatefulSets {
		actualReplicas := sset.GetReplicas(actualSset)
		expectedSset, shouldExist := expectedStatefulSets.GetByName(actualSset.Name)
		expectedReplicas := int32(0) // sset removal
		if shouldExist {             // sset downscale
			expectedReplicas = sset.GetReplicas(expectedSset)
		}
		if expectedReplicas == 0 || // removal
			expectedReplicas < actualReplicas { // downscale
			downscales = append(downscales, ssetDownscale{
				statefulSet:     actualSset,
				initialReplicas: actualReplicas,
				targetReplicas:  expectedReplicas,
			})
		}
	}
	return downscales
}

// scheduleDataMigrations requests Elasticsearch to migrate data away from leavingNodes.
// If leavingNodes is empty, it clears any existing settings.
func scheduleDataMigrations(esClient esclient.Client, leavingNodes []string) error {
	if len(leavingNodes) != 0 {
		log.V(1).Info("Migrating data away from nodes", "nodes", leavingNodes)
	}
	return migration.MigrateData(esClient, leavingNodes)
}

// attemptDownscale attempts to decrement the number of replicas of the given StatefulSet,
// or deletes the StatefulSet entirely if it should not contain any replica.
// Nodes whose data migration is not over will not be removed.
// A boolean is returned to indicate if a requeue should be scheduled if the entire downscale could not be performed.
func attemptDownscale(
	ctx downscaleContext,
	downscale ssetDownscale,
	state *downscaleState,
	allLeavingNodes []string,
	statefulSets sset.StatefulSetList,
) (bool, error) {
	switch {
	case downscale.isRemoval():
		ssetLogger(downscale.statefulSet).Info("Deleting statefulset")
		return false, ctx.k8sClient.Delete(&downscale.statefulSet)

	case downscale.isReplicaDecrease():
		// adjust the theoretical downscale to one we can safely perform
		performable := calculatePerformableDownscale(ctx, state, downscale, allLeavingNodes)
		if !performable.isReplicaDecrease() {
			// no downscale can be performed for now, let's requeue
			return true, nil
		}
		// do performable downscale, and requeue if needed
		shouldRequeue := performable.targetReplicas != downscale.targetReplicas
		return shouldRequeue, doDownscale(ctx, performable, statefulSets)

	default:
		// nothing to do
		return false, nil
	}
}

// calculatePerformableDownscale updates the given downscale target replicas to account for nodes
// which cannot be safely deleted yet.
// It returns the updated downscale and a boolean indicating whether a requeue should be done.
func calculatePerformableDownscale(
	ctx downscaleContext,
	state *downscaleState,
	downscale ssetDownscale,
	allLeavingNodes []string,
) ssetDownscale {
	// create another downscale based on the provided one, for which we'll slowly decrease target replicas
	performableDownscale := ssetDownscale{
		statefulSet:     downscale.statefulSet,
		initialReplicas: downscale.initialReplicas,
		targetReplicas:  downscale.initialReplicas, // target set to initial
	}
	// iterate on all leaving nodes (ordered by highest ordinal first)
	for _, node := range downscale.leavingNodeNames() {
		if canDownscale, reason := checkDownscaleInvariants(*state, downscale.statefulSet); !canDownscale {
			ssetLogger(downscale.statefulSet).V(1).Info("Cannot downscale StatefulSet", "node", node, "reason", reason)
			return performableDownscale
		}
		if migration.IsMigratingData(ctx.observedState, node, allLeavingNodes) {
			ssetLogger(downscale.statefulSet).V(1).Info("Data migration not over yet, skipping node deletion", "node", node)
			ctx.reconcileState.UpdateElasticsearchMigrating(ctx.resourcesState, ctx.observedState)
			// no need to check other nodes since we remove them in order and this one isn't ready anyway
			return performableDownscale
		}
		// data migration over: allow pod to be removed
		performableDownscale.targetReplicas--
		state.recordOneRemoval(downscale.statefulSet)
	}
	return performableDownscale
}

// doDownscale schedules nodes removal for the given downscale, and updates zen settings accordingly.
func doDownscale(downscaleCtx downscaleContext, downscale ssetDownscale, actualStatefulSets sset.StatefulSetList) error {
	ssetLogger(downscale.statefulSet).Info(
		"Scaling replicas down",
		"from", downscale.initialReplicas,
		"to", downscale.targetReplicas,
	)

	if err := updateZenSettingsForDownscale(
		downscaleCtx.k8sClient,
		downscaleCtx.esClient,
		downscaleCtx.es,
		downscaleCtx.reconcileState,
		actualStatefulSets,
		downscale.leavingNodeNames()...,
	); err != nil {
		return err
	}

	nodespec.UpdateReplicas(&downscale.statefulSet, &downscale.targetReplicas)
	if err := downscaleCtx.k8sClient.Update(&downscale.statefulSet); err != nil {
		return err
	}

	// Expect the updated statefulset in the cache for next reconciliation.
	downscaleCtx.expectations.ExpectGeneration(downscale.statefulSet.ObjectMeta)

	return nil
}

// updateZenSettingsForDownscale makes sure zen1 and zen2 settings are updated to account for nodes
// that will soon be removed.
func updateZenSettingsForDownscale(
	c k8s.Client,
	esClient esclient.Client,
	es v1alpha1.Elasticsearch,
	reconcileState *reconcile.State,
	actualStatefulSets sset.StatefulSetList,
	excludeNodes ...string,
) error {
	// Maybe update zen1 minimum_master_nodes.
	if err := maybeUpdateZen1ForDownscale(c, esClient, es, reconcileState, actualStatefulSets); err != nil {
		return err
	}

	// Maybe update zen2 settings to exclude leaving master nodes from voting.
	if err := zen2.AddToVotingConfigExclusions(c, esClient, es, excludeNodes); err != nil {
		return err
	}
	return nil
}

// maybeUpdateZen1ForDownscale updates zen1 minimum master nodes if we are downscaling from 2 to 1 master node.
func maybeUpdateZen1ForDownscale(
	c k8s.Client,
	esClient esclient.Client,
	es v1alpha1.Elasticsearch,
	reconcileState *reconcile.State,
	actualStatefulSets sset.StatefulSetList) error {
	// Check if we have at least one Zen1 compatible pod or StatefulSet in flight.
	if zen1compatible, err := zen1.AtLeastOneNodeCompatibleWithZen1(actualStatefulSets, c, es); !zen1compatible || err != nil {
		return err
	}

	actualMasters, err := sset.GetActualMastersForCluster(c, es)
	if err != nil {
		return err
	}
	if len(actualMasters) != 2 {
		// not in the 2->1 situation
		return nil
	}

	// We are moving from 2 to 1 master nodes, we need to update minimum_master_nodes before removing
	// the 2nd node, otherwise the cluster won't be able to form anymore.
	// This is inherently unsafe (can cause split brains), but there's no alternative.
	// For other situations (eg. 3 -> 2), it's fine to update minimum_master_nodes after the node is removed
	// (will be done at next reconciliation, before nodes removal).
	reconcileState.AddEvent(
		v1.EventTypeWarning, events.EventReasonUnhealthy,
		"Downscaling from 2 to 1 master nodes: unsafe operation",
	)
	minimumMasterNodes := 1
	//TODO: we should also update m_m_n in the configuration file, otherwise if we are unlucky the remaining master
	// may restart and wait for 2 masters.
	return zen1.UpdateMinimumMasterNodesTo(es, esClient, reconcileState, minimumMasterNodes)
}
