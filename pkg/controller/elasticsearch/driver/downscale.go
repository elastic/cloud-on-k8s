// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"errors"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// HandleDownscale attempts to downscale actual StatefulSets towards expected ones.
func HandleDownscale(
	downscaleCtx downscaleContext,
	expectedStatefulSets sset.StatefulSetList,
	actualStatefulSets sset.StatefulSetList,
) *reconciler.Results {
	results := &reconciler.Results{}

	// Retrieve the current list of Pods for this cluster. This list is used to compute the nodes that should be eventually removed,
	// and the ones that will be removed in this reconciliation attempt.
	actualPods, err := sset.GetActualPodsForCluster(downscaleCtx.k8sClient, downscaleCtx.es)
	if err != nil {
		return results.WithError(err)
	}

	// Compute the desired downscale, without applying any budget filter, to feed the status and let the user know what nodes should
	// be eventually removed, not only in this reconciliation attempt, but also in the next ones.
	desiredDownscale, _ := podsToDownscale(actualPods, downscaleCtx.es, expectedStatefulSets, actualStatefulSets, noDownscaleFilter)
	desiredLeavingNodes := leavingNodeNames(desiredDownscale)
	downscaleCtx.reconcileState.RecordNodesToBeRemoved(desiredLeavingNodes)

	// Compute the desired downscale, applying a budget filter to make sure we only downscale nodes we're allowed to.
	downscaleState := newDownscaleState(actualPods, downscaleCtx.es)

	// compute the list of StatefulSet downscales and deletions to perform
	downscales, deletions := calculateDownscales(*downscaleState, expectedStatefulSets, actualStatefulSets, downscaleBudgetFilter)

	// remove actual StatefulSets that should not exist anymore (already downscaled to 0 in the past)
	// this is safe thanks to expectations: we're sure 0 actual replicas means 0 corresponding pods exist
	if err := deleteStatefulSets(deletions, downscaleCtx.k8sClient, downscaleCtx.es); err != nil {
		return results.WithError(err)
	}

	// initiate shutdown of nodes that should be removed
	// if leaving nodes is empty this should cancel any ongoing shutdowns
	leavingNodes := leavingNodeNames(downscales)
	if err := downscaleCtx.nodeShutdown.ReconcileShutdowns(downscaleCtx.parentCtx, leavingNodes); err != nil {
		return results.WithError(err)
	}

	for _, downscale := range downscales {
		// attempt the StatefulSet downscale (may or may not remove nodes)
		requeue, err := attemptDownscale(downscaleCtx, downscale, actualStatefulSets)
		if err != nil {
			return results.WithError(err)
		}
		if requeue {
			// retry downscaling this statefulset later
			results.WithReconciliationState(defaultRequeue.WithReason("Downscale in progress"))
		}
	}

	// Ensure that the status mention the delayed nodes
	if delayedLeavingNodes, _ := stringsutil.Difference(desiredLeavingNodes, leavingNodes); len(delayedLeavingNodes) > 0 {
		sort.Strings(delayedLeavingNodes)
		results.WithReconciliationState(defaultRequeue.WithReason(fmt.Sprintf("Downscale in progress, delayed nodes: %s", delayedLeavingNodes)))
	}
	return results
}

func podsToDownscale(
	actualPods []corev1.Pod,
	es esv1.Elasticsearch,
	expectedStatefulSets sset.StatefulSetList,
	actualStatefulSets sset.StatefulSetList,
	downscaleFilter downscaleFilter,
) ([]ssetDownscale, sset.StatefulSetList) {
	downscaleState := newDownscaleState(actualPods, es)
	// compute the list of StatefulSet downscales and deletions to perform
	return calculateDownscales(*downscaleState, expectedStatefulSets, actualStatefulSets, downscaleFilter)
}

// deleteStatefulSets deletes the given StatefulSets along with their associated resources.
func deleteStatefulSets(toDelete sset.StatefulSetList, k8sClient k8s.Client, es esv1.Elasticsearch) error {
	for _, toDelete := range toDelete {
		if err := deleteStatefulSetResources(k8sClient, es, toDelete); err != nil {
			return err
		}
	}
	return nil
}

// calculateDownscales compares expected and actual StatefulSets to return a list of StatefulSets
// that can be downscaled (replica decrease) or deleted (no replicas).
func calculateDownscales(
	state downscaleState,
	expectedStatefulSets sset.StatefulSetList,
	actualStatefulSets sset.StatefulSetList,
	downscaleFilter downscaleFilter,
) (downscales []ssetDownscale, deletions sset.StatefulSetList) {
	expectedStatefulSetsNames := expectedStatefulSets.Names()
	for _, actualSset := range actualStatefulSets {
		actualReplicas := sset.GetReplicas(actualSset)
		expectedSset, shouldExist := expectedStatefulSets.GetByName(actualSset.Name)
		expectedReplicas := int32(0)
		if shouldExist {
			expectedReplicas = sset.GetReplicas(expectedSset)
		}

		switch {
		case !expectedStatefulSetsNames.Has(actualSset.Name) && actualReplicas == 0 && expectedReplicas == 0:
			// the StatefulSet should not exist, and currently has no replicas
			// it is safe to delete
			deletions = append(deletions, actualSset)

		case expectedReplicas < actualReplicas:
			// the StatefulSet should be downscaled
			requestedDeletes := actualReplicas - expectedReplicas
			allowedDeletes := downscaleFilter(&state, actualSset, requestedDeletes)
			if allowedDeletes == 0 {
				continue
			}
			downscales = append(downscales, ssetDownscale{
				statefulSet:     actualSset,
				initialReplicas: actualReplicas,
				targetReplicas:  actualReplicas - allowedDeletes,
				finalReplicas:   expectedReplicas,
			})

		default:
			// nothing to do
		}
	}
	return downscales, deletions
}

type downscaleFilter func(_ *downscaleState, _ appsv1.StatefulSet, _ int32) int32

// noDownscaleFilter is a filter which does not remove any Pod. It can be used to compute the full list of
// Pods which are expected to be deleted.
func noDownscaleFilter(_ *downscaleState, _ appsv1.StatefulSet, requestedDeletes int32) int32 {
	return requestedDeletes
}

// downscaleBudgetFilter is a filter which relies on checkDownscaleInvariants.
// It ensures that we only downscale nodes we're allowed to.
// Note that this function may have side effects on the downscaleState and should not be considered as idempotent.
func downscaleBudgetFilter(state *downscaleState, actualSset appsv1.StatefulSet, requestedDeletes int32) int32 {
	allowedDeletes, reason := checkDownscaleInvariants(*state, actualSset, requestedDeletes)
	if allowedDeletes == 0 {
		ssetLogger(actualSset).V(1).Info("Cannot downscale StatefulSet", "reason", reason)
		return 0
	}
	state.recordNodeRemoval(actualSset, allowedDeletes)
	return allowedDeletes
}

// attemptDownscale attempts to decrement the number of replicas of the given StatefulSet.
// Nodes whose data migration is not over will not be removed.
// A boolean is returned to indicate if a requeue should be scheduled if the entire downscale could not be performed.
func attemptDownscale(
	ctx downscaleContext,
	downscale ssetDownscale,
	statefulSets sset.StatefulSetList,
) (bool, error) {
	// adjust the theoretical downscale to one we can safely perform
	performable, err := calculatePerformableDownscale(ctx, downscale)
	if err != nil {
		return true, err
	}
	if performable.targetReplicas == performable.initialReplicas {
		// no downscale can be performed for now, let's requeue
		return true, nil
	}
	// do performable downscale, and requeue if needed
	shouldRequeue := performable.targetReplicas != downscale.finalReplicas
	return shouldRequeue, doDownscale(ctx, performable, statefulSets)
}

// deleteStatefulSetResources deletes the given StatefulSet along with the corresponding
// headless service, configuration and transport certificates secret.
func deleteStatefulSetResources(k8sClient k8s.Client, es esv1.Elasticsearch, statefulSet appsv1.StatefulSet) error {
	headlessSvc := nodespec.HeadlessService(&es, statefulSet.Name)
	err := k8sClient.Delete(context.Background(), &headlessSvc)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	err = settings.DeleteConfig(k8sClient, es.Namespace, statefulSet.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	err = transport.DeleteStatefulSetTransportCertificate(k8sClient, es.Namespace, statefulSet.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	ssetLogger(statefulSet).Info("Deleting statefulset")
	return k8sClient.Delete(context.Background(), &statefulSet)
}

// calculatePerformableDownscale updates the given downscale target replicas to account for nodes
// which cannot be safely deleted yet.
// It returns the updated downscale and a boolean indicating whether a requeue should be done.
func calculatePerformableDownscale(
	ctx downscaleContext,
	downscale ssetDownscale,
) (ssetDownscale, error) {
	// create another downscale based on the provided one, for which we'll slowly decrease target replicas
	performableDownscale := ssetDownscale{
		statefulSet:     downscale.statefulSet,
		initialReplicas: downscale.initialReplicas,
		targetReplicas:  downscale.initialReplicas, // target set to initial
		finalReplicas:   downscale.finalReplicas,
	}
	// iterate on all leaving nodes (ordered by highest ordinal first)
	for _, node := range downscale.leavingNodeNames() {
		response, err := ctx.nodeShutdown.ShutdownStatus(ctx.parentCtx, node)
		if err != nil {
			return performableDownscale, fmt.Errorf("while checking shutdown status: %w", err)
		}
		switch response.Status {
		case esclient.ShutdownComplete:
			// shutdown including data migration over: allow pod to be removed
			performableDownscale.targetReplicas--
		case esclient.ShutdownStalled:
			// shutdown stalled this can require user interaction: bubble up via event
			ctx.reconcileState.
				UpdateWithPhase(esv1.ElasticsearchNodeShutdownStalledPhase).
				AddEvent(
					corev1.EventTypeWarning,
					events.EventReasonStalled,
					fmt.Sprintf("Requested topology change is stalled. User intervention maybe required if this condition persists. %s", response.Explanation),
				)
			// no need to check other nodes since we remove them in order and this one isn't ready anyway
			return performableDownscale, nil
		case esclient.ShutdownInProgress:
			ctx.reconcileState.
				UpdateWithPhase(esv1.ElasticsearchMigratingDataPhase).
				AddEvent(
					corev1.EventTypeNormal,
					events.EventReasonDelayed,
					"Requested topology change delayed by data migration. Ensure index settings allow node removal.",
				)
			// no need to check other nodes since we remove them in order and this one isn't ready anyway
			return performableDownscale, nil
		case esclient.ShutdownNotStarted:
			msg := fmt.Sprintf("Unexpected state. Node shutdown could not be started: %s", response.Explanation)
			return performableDownscale, errors.New(msg)
		}
	}
	return performableDownscale, nil
}

// doDownscale schedules nodes removal for the given downscale, and updates zen settings accordingly.
func doDownscale(downscaleCtx downscaleContext, downscale ssetDownscale, actualStatefulSets sset.StatefulSetList) error {
	ssetLogger(downscale.statefulSet).Info(
		"Scaling replicas down",
		"from", downscale.initialReplicas,
		"to", downscale.targetReplicas,
	)

	if label.IsMasterNodeSet(downscale.statefulSet) {
		if err := updateZenSettingsForDownscale(
			downscaleCtx.parentCtx,
			downscaleCtx.k8sClient,
			downscaleCtx.esClient,
			downscaleCtx.es,
			downscaleCtx.reconcileState,
			actualStatefulSets,
			downscale.leavingNodeNames()...,
		); err != nil {
			return err
		}
	}

	nodespec.UpdateReplicas(&downscale.statefulSet, &downscale.targetReplicas)
	if err := downscaleCtx.k8sClient.Update(context.Background(), &downscale.statefulSet); err != nil {
		return err
	}

	// Expect the updated statefulset in the cache for next reconciliation.
	downscaleCtx.expectations.ExpectGeneration(downscale.statefulSet)

	return nil
}

// updateZenSettingsForDownscale makes sure zen1 and zen2 settings are updated to account for nodes
// that will soon be removed.
func updateZenSettingsForDownscale(
	ctx context.Context,
	c k8s.Client,
	esClient esclient.Client,
	es esv1.Elasticsearch,
	reconcileState *reconcile.State,
	actualStatefulSets sset.StatefulSetList,
	excludeNodes ...string,
) error {
	// Maybe update zen1 minimum_master_nodes.
	if err := maybeUpdateZen1ForDownscale(ctx, c, esClient, es, reconcileState, actualStatefulSets); err != nil {
		return err
	}

	// Maybe update zen2 settings to exclude leaving master nodes from voting.
	return zen2.AddToVotingConfigExclusions(ctx, c, esClient, es, excludeNodes)
}

// maybeUpdateZen1ForDownscale updates zen1 minimum master nodes if we are downscaling from 2 to 1 master node.
func maybeUpdateZen1ForDownscale(
	ctx context.Context,
	c k8s.Client,
	esClient esclient.Client,
	es esv1.Elasticsearch,
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
		corev1.EventTypeWarning, events.EventReasonUnhealthy,
		"Downscaling from 2 to 1 master nodes: unsafe operation",
	)
	minimumMasterNodes := 1
	return zen1.UpdateMinimumMasterNodesTo(ctx, es, esClient, minimumMasterNodes)
}
