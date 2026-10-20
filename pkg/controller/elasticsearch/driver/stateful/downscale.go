// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates/transport"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/stringsutil"
)

// HandleDownscale attempts to downscale actual StatefulSets towards expected ones.
func HandleDownscale(
	downscaleCtx downscaleContext,
	expectedStatefulSets es_sset.StatefulSetList,
	actualStatefulSets es_sset.StatefulSetList,
	leaving *leavingNodes,
) *reconciler.Results {
	results := &reconciler.Results{}

	// Retrieve the current list of Pods for this cluster. This list is used to compute the nodes that should be eventually removed,
	// and the ones that will be removed in this reconciliation attempt.
	actualPods, err := es_sset.GetActualPodsForCluster(downscaleCtx.k8sClient, downscaleCtx.es)
	if err != nil {
		return results.WithError(err)
	}

	desiredLeavingNodes := leaving.names()

	// Retrieve the desired leaving nodes that are draining, ready or not, see newDownscaleState. Nodes that stay are
	// ignored, so that a shutdown registered outside of the operator does not allow more removals.
	draining, err := leaving.draining(downscaleCtx.parentCtx, desiredLeavingNodes)
	if err != nil {
		return results.WithError(err)
	}

	// The leaving nodes that are not members of the cluster cannot be shut down. They are held, not removed, as they may
	// still hold data, and do not fail the downscale: the next steps can recreate them, for them to join the cluster.
	notMembers, err := nodesNotInCluster(downscaleCtx, leaving)
	if err != nil {
		return results.WithError(err)
	}

	// Feed the status with the desired leaving nodes, computed without applying any budget filter, to let the user know what
	// nodes should be eventually removed, not only in this reconciliation attempt, but also in the next ones. They are
	// only recorded once their shutdowns are retrieved: recording them resets their shutdown status, which the forced
	// upgrade relies on while Elasticsearch is unreachable, so a failed retrieval must keep the reported one.
	downscaleCtx.reconcileState.RecordNodesToBeRemoved(desiredLeavingNodes)

	// Compute the desired downscale, applying a budget filter to make sure we only downscale nodes we're allowed to.
	downscaleState := newDownscaleState(actualPods, actualStatefulSets, downscaleCtx.es, draining, notMembers)

	// compute the list of StatefulSet downscales and deletions to perform
	downscales, deletions, heldReasons := calculateDownscales(downscaleCtx.parentCtx, *downscaleState, expectedStatefulSets, actualStatefulSets, downscaleBudgetFilter)

	// remove actual StatefulSets that should not exist anymore (already downscaled to 0 in the past)
	// this is safe thanks to expectations: we're sure 0 actual replicas means 0 corresponding pods exist
	if err := deleteStatefulSets(downscaleCtx.parentCtx, deletions, downscaleCtx.k8sClient, downscaleCtx.es); err != nil {
		return results.WithError(err)
	}

	// request the shutdowns of the nodes to remove, and keep those of the draining nodes even if they cannot be removed
	// now: cancelling them would make the nodes ready only to drain them again. Cancel the shutdowns of the other
	// non-terminating nodes. While a leaving node is not a member of the cluster, the shutdowns of the nodes that
	// are not members are kept: it may have left the cluster while draining, for example because its Pod restarted, and
	// its shutdown cannot be told apart from the others as its node ID is unknown.
	selectedNodes := leavingNodeNames(downscales)
	keptNodes := draining.Diff(set.Make(selectedNodes...)).AsSortedSlice()
	shutdownNodes := slices.Concat(selectedNodes, keptNodes)
	terminatingNodes := k8s.PodNames(k8s.TerminatingPods(actualPods))
	requeue, err := requestShutdownsAndDownscale(downscaleCtx, downscales, shutdownNodes, terminatingNodes, len(notMembers) > 0)
	if err != nil {
		return results.WithError(err)
	}

	// Report the delayed nodes and why they are held in a single reason, as only the first one of a priority is kept.
	reason := "Downscale in progress"
	if delayedLeavingNodes, _ := stringsutil.Difference(desiredLeavingNodes, selectedNodes); len(delayedLeavingNodes) > 0 {
		sort.Strings(delayedLeavingNodes)
		// the held reasons are only reported with delayed nodes: a downscale is only held when fewer nodes than
		// requested are removed, which delays the others
		reason = fmt.Sprintf("%s, delayed nodes: %s%s", reason, delayedLeavingNodes, formatHeldReasons(heldReasons))
		requeue = true
	}
	if requeue {
		results.WithReconciliationState(shared.DefaultRequeue.WithReason(reason))
	}
	return results
}

// requestShutdownsAndDownscale requests the shutdowns of shutdownNodes and attempts the downscales, then cancels the
// shutdowns of the other non-terminating nodes, even if an earlier step failed. It returns whether a requeue should be
// scheduled if some of the downscales could not be performed entirely.
func requestShutdownsAndDownscale(
	downscaleCtx downscaleContext,
	downscales []ssetDownscale,
	shutdownNodes, terminatingNodes []string,
	keepNonMembers bool,
) (bool, error) {
	requeue, err := downscaleAfterShutdowns(downscaleCtx, downscales, shutdownNodes)
	// Nodes that are no longer leaving, for example because of a later spec change, would otherwise keep their shutdown
	// for as long as another node is held draining, using up the downscale budget and blocking the rolling upgrade.
	// Cancelling their shutdowns does not depend on the nodes to shut down, so it is done whether or not their shutdowns
	// could be requested, and after the downscales, whether they failed or not. Its failure is only returned: it must not
	// hide the shutdown statuses, nor the stalled one, nor hold the downscales back.
	cancelErr := downscaleCtx.nodeShutdown.CancelShutdownsExcept(downscaleCtx.parentCtx, shutdownNodes, terminatingNodes, keepNonMembers)
	// the same failure, for example if the shutdowns cannot be retrieved, is only returned once
	if errors.Is(cancelErr, err) {
		cancelErr = nil
	}
	return requeue, errors.Join(err, cancelErr)
}

// downscaleAfterShutdowns requests the shutdowns of shutdownNodes, then attempts the downscales. It stops at the first
// failure. It returns whether a requeue should be scheduled if some of the downscales could not be performed entirely.
func downscaleAfterShutdowns(downscaleCtx downscaleContext, downscales []ssetDownscale, shutdownNodes []string) (bool, error) {
	if err := downscaleCtx.nodeShutdown.ReconcileShutdowns(downscaleCtx.parentCtx, shutdownNodes); err != nil {
		return false, err
	}
	statuses, err := shutdownStatuses(downscaleCtx, shutdownNodes)
	if err != nil {
		return false, err
	}
	// report a stalled shutdown after the downscales, so that it takes precedence over data migration, even if a
	// downscale fails
	defer reportShutdownStalled(downscaleCtx, statuses)
	return attemptDownscales(downscaleCtx, downscales, statuses)
}

// attemptDownscales attempts the given downscales in order, and stops at the first error. A boolean is returned to
// indicate if a requeue should be scheduled if some of them could not be performed entirely.
// statuses are the shutdown statuses returned by shutdownStatuses.
func attemptDownscales(
	ctx downscaleContext,
	downscales []ssetDownscale,
	statuses map[string]shutdown.NodeShutdownStatus,
) (bool, error) {
	var requeue bool
	for _, downscale := range downscales {
		// attempt the StatefulSet downscale (may or may not remove nodes)
		downscaleRequeue, err := attemptDownscale(ctx, downscale, statuses)
		if err != nil {
			return requeue, err
		}
		// retry downscaling this statefulset later
		requeue = requeue || downscaleRequeue
	}
	return requeue, nil
}

// formatHeldReasons formats the reasons why the downscale of StatefulSets is held, sorted by StatefulSet name.
func formatHeldReasons(heldReasons map[string]string) string {
	var sb strings.Builder
	for _, name := range slices.Sorted(maps.Keys(heldReasons)) {
		fmt.Fprintf(&sb, ", %s: %s", name, heldReasons[name])
	}
	return sb.String()
}

// nodesNotInCluster returns the desired leaving nodes that are not members of the cluster. Without node shutdown,
// membership does not matter, as a node is removed once it holds no shard: none is returned.
func nodesNotInCluster(downscaleCtx downscaleContext, leaving *leavingNodes) (set.StringSet, error) {
	if !supportsNodeShutdown(downscaleCtx.esClient.Version()) {
		return nil, nil
	}
	members, err := leaving.members(leaving.names())
	if err != nil {
		return nil, err
	}
	return leaving.desired.Diff(members), nil
}

// shutdownStatuses returns the shutdown status of the given nodes that have a shutdown registered, by node name, which
// also reports them in the downscale status through the observer, whether or not the nodes are removed. Shard migration
// registers no shutdown, its statuses are retrieved by calculatePerformableDownscale.
func shutdownStatuses(downscaleCtx downscaleContext, nodes []string) (map[string]shutdown.NodeShutdownStatus, error) {
	withShutdown, err := downscaleCtx.nodeShutdown.NodesWithShutdown(downscaleCtx.parentCtx, nodes)
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]shutdown.NodeShutdownStatus, len(withShutdown))
	for _, node := range withShutdown.AsSortedSlice() {
		status, err := downscaleCtx.nodeShutdown.ShutdownStatus(downscaleCtx.parentCtx, node)
		if err != nil {
			return nil, fmt.Errorf("while checking shutdown status: %w", err)
		}
		statuses[node] = status
	}
	return statuses, nil
}

// reportShutdownStalled reports the first stalled shutdown of the given statuses, in node name order, through the phase
// and an event, as it can require user interaction.
func reportShutdownStalled(downscaleCtx downscaleContext, statuses map[string]shutdown.NodeShutdownStatus) {
	for _, node := range slices.Sorted(maps.Keys(statuses)) {
		if statuses[node].Status != esclient.ShutdownStalled {
			continue
		}
		downscaleCtx.reconcileState.
			UpdateWithPhase(esv1.ElasticsearchNodeShutdownStalledPhase).
			AddEvent(
				corev1.EventTypeWarning,
				events.EventReasonStalled,
				events.EventActionShutdown,
				fmt.Sprintf("Requested topology change is stalled. User intervention maybe required if this condition persists. %s", statuses[node].Explanation),
			)
		return
	}
}

// deleteStatefulSets deletes the given StatefulSets along with their associated resources.
func deleteStatefulSets(ctx context.Context, toDelete es_sset.StatefulSetList, k8sClient k8s.Client, es esv1.Elasticsearch) error {
	for _, toDelete := range toDelete {
		if err := deleteStatefulSetResources(ctx, k8sClient, es, toDelete); err != nil {
			return err
		}
	}
	return nil
}

// calculateDownscales compares expected and actual StatefulSets to return a list of StatefulSets
// that can be downscaled (replica decrease) or deleted (no replicas), and why the held downscales are held, by
// StatefulSet name.
func calculateDownscales(ctx context.Context, state downscaleState, expectedStatefulSets es_sset.StatefulSetList, actualStatefulSets es_sset.StatefulSetList, downscaleFilter downscaleFilter) (downscales []ssetDownscale, deletions es_sset.StatefulSetList, heldReasons map[string]string) {
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
			allowedDeletes, heldReason := downscaleFilter(ctx, &state, actualSset, requestedDeletes)
			if heldReason != "" {
				if heldReasons == nil {
					heldReasons = map[string]string{}
				}
				heldReasons[actualSset.Name] = heldReason
			}
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
	return downscales, deletions, heldReasons
}

// downscaleFilter returns how many of the requested deletes are allowed, and the reason why the others are held.
type downscaleFilter func(_ context.Context, _ *downscaleState, _ appsv1.StatefulSet, _ int32) (int32, string)

// noDownscaleFilter is a filter which does not remove any Pod. It can be used to compute the full list of
// Pods which are expected to be deleted.
func noDownscaleFilter(_ context.Context, _ *downscaleState, _ appsv1.StatefulSet, requestedDeletes int32) (int32, string) {
	return requestedDeletes, ""
}

// downscaleBudgetFilter is a filter which relies on checkDownscaleInvariants.
// It ensures that we only downscale nodes we're allowed to.
// Note that this function may have side effects on the downscaleState and should not be considered as idempotent.
func downscaleBudgetFilter(ctx context.Context, state *downscaleState, actualSset appsv1.StatefulSet, requestedDeletes int32) (int32, string) {
	removals, reason := checkDownscaleInvariants(*state, actualSset, requestedDeletes)
	if removals.count == 0 {
		ssetLogger(ctx, actualSset).V(1).Info("Cannot downscale StatefulSet", "reason", reason)
		return 0, reason
	}
	state.recordNodeRemoval(actualSset, removals)
	return removals.count, reason
}

// attemptDownscale attempts to decrement the number of replicas of the given StatefulSet.
// Nodes whose data migration is not over will not be removed.
// A boolean is returned to indicate if a requeue should be scheduled if the entire downscale could not be performed.
// statuses are the shutdown statuses returned by shutdownStatuses.
func attemptDownscale(
	ctx downscaleContext,
	downscale ssetDownscale,
	statuses map[string]shutdown.NodeShutdownStatus,
) (bool, error) {
	// adjust the theoretical downscale to one we can safely perform
	performable, err := calculatePerformableDownscale(ctx, downscale, statuses)
	if err != nil {
		return true, err
	}
	if performable.targetReplicas == performable.initialReplicas {
		// no downscale can be performed for now, let's requeue
		return true, nil
	}
	// do performable downscale, and requeue if needed
	shouldRequeue := performable.targetReplicas != downscale.finalReplicas
	return shouldRequeue, doDownscale(ctx, performable)
}

// deleteStatefulSetResources deletes the given StatefulSet along with the corresponding
// headless service, configuration and transport certificates secret.
func deleteStatefulSetResources(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, statefulSet appsv1.StatefulSet) error {
	headlessSvc := corev1.Service{
		Namespace: es.Namespace,
		Name:      nodespec.HeadlessServiceName(statefulSet.Name),
	}
	err := k8sClient.Delete(ctx, &headlessSvc)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	err = settings.DeleteConfig(ctx, k8sClient, es.Namespace, statefulSet.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	err = transport.DeleteStatefulSetTransportCertificate(ctx, k8sClient, es.Namespace, statefulSet.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	ssetLogger(ctx, statefulSet).Info("Deleting statefulset")
	return k8sClient.Delete(ctx, &statefulSet)
}

// calculatePerformableDownscale updates the given downscale target replicas to account for nodes
// which cannot be safely deleted yet.
// It returns the updated downscale and a boolean indicating whether a requeue should be done.
// statuses are the shutdown statuses returned by shutdownStatuses.
func calculatePerformableDownscale(
	ctx downscaleContext,
	downscale ssetDownscale,
	statuses map[string]shutdown.NodeShutdownStatus,
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
		response, ok := statuses[node]
		if !ok {
			// with shard migration, retrieving a status requires requests, so it is only done for the inspected nodes
			var err error
			response, err = ctx.nodeShutdown.ShutdownStatus(ctx.parentCtx, node)
			if err != nil {
				return performableDownscale, fmt.Errorf("while checking shutdown status: %w", err)
			}
		}
		switch response.Status {
		case esclient.ShutdownComplete:
			// shutdown including data migration over: allow pod to be removed
			performableDownscale.targetReplicas--
		case esclient.ShutdownStalled:
			// reported by reportShutdownStalled
			// no need to check other nodes since we remove them in order and this one isn't ready anyway
			return performableDownscale, nil
		case esclient.ShutdownInProgress:
			ctx.reconcileState.
				UpdateWithPhase(esv1.ElasticsearchMigratingDataPhase).
				AddEvent(
					corev1.EventTypeNormal,
					events.EventReasonDelayed,
					events.EventActionShutdown,
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
func doDownscale(downscaleCtx downscaleContext, downscale ssetDownscale) error {
	ssetLogger(downscaleCtx.parentCtx, downscale.statefulSet).Info(
		"Scaling replicas down",
		"from", downscale.initialReplicas,
		"to", downscale.targetReplicas,
		"nodes", downscale.leavingNodeNames(),
	)

	if label.IsMasterNodeSet(downscale.statefulSet) {
		// Update zen2 settings to exclude leaving master nodes from voting.
		if err := zen2.AddToVotingConfigExclusions(
			downscaleCtx.parentCtx,
			downscaleCtx.esClient,
			downscaleCtx.es,
			downscale.leavingNodeNames(),
		); err != nil {
			return err
		}
	}

	nodespec.UpdateReplicas(&downscale.statefulSet, &downscale.targetReplicas)
	if err := downscaleCtx.k8sClient.Update(downscaleCtx.parentCtx, &downscale.statefulSet); err != nil {
		return err
	}

	// Expect the updated statefulset in the cache for next reconciliation.
	downscaleCtx.expectations.ExpectGeneration(&downscale.statefulSet)

	return nil
}
