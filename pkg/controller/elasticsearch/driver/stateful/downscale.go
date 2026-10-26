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
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"

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
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
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

	// The leaving nodes that are not members of the cluster cannot be shut down. They are held without failing the
	// downscale, as they may hold data and the next steps may make them join, unless classifyNonMembers says otherwise.
	notMembers, err := nodesNotInCluster(downscaleCtx, leaving)
	if err != nil {
		return results.WithError(err)
	}

	// Feed the status with the desired leaving nodes, computed without applying any budget filter, to let the user know what
	// nodes should be eventually removed, not only in this reconciliation attempt, but also in the next ones. They are
	// only recorded once their shutdowns are retrieved: recording them resets their shutdown status, which the forced
	// upgrade relies on while Elasticsearch is unreachable, so a failed retrieval must keep the reported one.
	downscaleCtx.reconcileState.RecordNodesToBeRemoved(desiredLeavingNodes)

	// Report a stalled shutdown after the downscales, so that it takes precedence over data migration, even if a
	// downscale fails or a shutdown cannot be requested: the current statuses are added to stalledCandidates once they
	// are known.
	stalledCandidates := make(map[string]shutdown.NodeShutdownStatus)
	// the statuses of the unreached nodes come from a previous reconciliation: they come after the current ones
	unreachedStatuses := make(map[string]shutdown.NodeShutdownStatus)
	defer reportShutdownStalled(downscaleCtx, stalledCandidates, unreachedStatuses)

	// Recording the nodes resets the shutdown status of the draining ones: report it again right away, from the retrieved
	// shutdowns. If a step fails before they are requested, for example the deletion of a StatefulSet, the reset status
	// would be saved, and the forced upgrade relies on it while Elasticsearch is not reachable.
	drainingStatuses, err := shutdownStatuses(downscaleCtx, sets.List(draining))
	if err != nil {
		return results.WithError(err)
	}
	maps.Copy(stalledCandidates, drainingStatuses)

	// The leaving nodes that are missing from a partial nodes response have no node ID: no shutdown is retrieved or
	// requested for them, so nothing else reports them. They keep the shutdown status they were reported with, for the
	// upgrades to keep leaving them alone in the next reconciliations too, as long as the response is partial. Their
	// status is reported as stalled too, so that the phase and the event match it, like those of the other nodes, in every
	// reconciliation: the explanation is the one of the last reconciliation that reached Elasticsearch, so it may be
	// out of date. A stalled shutdown that is current is reported before it.
	if supportsNodeShutdown(downscaleCtx.esClient.Version()) {
		unreached, err := leaving.unreached(desiredLeavingNodes)
		if err != nil {
			return results.WithError(err)
		}
		for _, node := range sets.List(unreached) {
			unreachedStatuses[node] = leaving.reportedStatus(node)
			downscaleCtx.reconcileState.OnShutdownStatus(node, unreachedStatuses[node])
		}
	}

	// Compute the desired downscale, applying a budget filter to make sure we only downscale nodes we're allowed to.
	downscaleState := newDownscaleState(actualPods, actualStatefulSets, downscaleCtx.es, draining)
	if notMembers.Len() > 0 {
		// a node that is not a member may be a member that did not answer, the reason why it is held says so
		if downscaleState.partialNodes, err = leaving.esState.NodesResponsePartial(); err != nil {
			return results.WithError(err)
		}
		downscaleState.nonMembers = classifyNonMembers(notMembers, expectedStatefulSets, actualStatefulSets, actualPods)
		// a node that cannot join is only removed from a settled cluster, which is checked at most once, and only when
		// such a node is the next to be removed
		if !downscaleState.partialNodes {
			downscaleState.nonMemberRemovalsAllowed = sync.OnceValue(func() bool {
				return clusterIsSettled(downscaleCtx.parentCtx, downscaleCtx.esClient, leaving.esState)
			})
		}
	}

	// compute the list of StatefulSet downscales and deletions to perform
	downscales, deletions, heldReasons := calculateDownscales(downscaleCtx.parentCtx, *downscaleState,
		expectedReplicasOfStatefulSets(expectedStatefulSets), actualStatefulSets, downscaleBudgetFilter)

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
	keptNodes := sets.List(draining.Difference(sets.New(selectedNodes...)))
	// the removable nodes that are not members of the cluster have no shutdown: none is requested for them
	shutdownNodes := slices.Concat(
		slices.DeleteFunc(slices.Clone(selectedNodes), downscaleState.isRemovableNonMember),
		keptNodes,
	)
	terminatingNodes := k8s.PodNames(k8s.TerminatingPods(actualPods))
	requeue, err := requestShutdownsAndDownscale(downscaleCtx, downscales, shutdownNodes, terminatingNodes, len(notMembers) > 0, stalledCandidates)
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
// scheduled if some of the downscales could not be performed entirely. The shutdown statuses that are known are added
// to stalledCandidates, even on failure, for a stalled one to be reported.
func requestShutdownsAndDownscale(
	downscaleCtx downscaleContext,
	downscales []ssetDownscale,
	shutdownNodes, terminatingNodes []string,
	keepNonMembers bool,
	stalledCandidates map[string]shutdown.NodeShutdownStatus,
) (bool, error) {
	requeue, err := downscaleAfterShutdowns(downscaleCtx, downscales, shutdownNodes, stalledCandidates)
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
// The shutdown statuses that are known are added to stalledCandidates, even on failure.
func downscaleAfterShutdowns(
	downscaleCtx downscaleContext,
	downscales []ssetDownscale,
	shutdownNodes []string,
	stalledCandidates map[string]shutdown.NodeShutdownStatus,
) (bool, error) {
	requestErr := downscaleCtx.nodeShutdown.ReconcileShutdowns(downscaleCtx.parentCtx, shutdownNodes)
	// the shutdowns that were confirmed may be stalled, even if some of them could not be requested
	statuses, statusErr := shutdownStatuses(downscaleCtx, shutdownNodes)
	if statusErr == nil {
		maps.Copy(stalledCandidates, statuses)
	}
	if requestErr != nil {
		return false, requestErr
	}
	if statusErr != nil {
		return false, statusErr
	}
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

// classifyNonMembers returns the kind of each of the given nodes, which are not members of the cluster, by name. A node
// is nonMemberRecoverable if the next steps may make it join: its Pod is expected, exists and is scheduled, so it may be
// starting, or be a member that restarts. Otherwise, as nothing repairs a Pod of a removed StatefulSet, recreates a
// missing Pod, or reliably upgrades an unschedulable one, it is nonMemberUnrecoverable. Its removal waits for the cluster
// to be settled, whether it ever ran or not, as it may have: it may have been a member, for example recreated after the
// loss of its Kubernetes node, its claim may be bound to a retained volume, and the ephemeral volumes of a crash looping
// member survive container restarts. Whatever keeps it from running, a resource quota or a volume bound to a lost
// Kubernetes node for example, may never be resolved, and a settled cluster does not need its storage. The volume that
// may hold data, if any, is named for the user, see volumeWithData.
func classifyNonMembers(
	nodes sets.Set[string],
	expected, actual es_sset.StatefulSetList,
	pods []corev1.Pod,
) map[string]nonMember {
	podsByName := make(map[string]corev1.Pod, len(pods))
	for _, pod := range pods {
		podsByName[pod.Name] = pod
	}
	nonMembers := make(map[string]nonMember, nodes.Len())
	for node := range nodes {
		nonMembers[node] = nonMember{kind: nonMemberRecoverable}
	}
	for _, statefulSet := range actual {
		_, stillExpected := expected.GetByName(statefulSet.Name)
		for ordinal := range sset.GetReplicas(statefulSet) {
			podName := sset.PodName(statefulSet.Name, ordinal)
			if !nodes.Has(podName) {
				continue
			}
			pod, exists := podsByName[podName]
			if stillExpected && exists && !isUnschedulable(pod) {
				continue
			}
			var existing *corev1.Pod
			if exists {
				existing = &pod
			}
			nonMembers[podName] = nonMember{kind: nonMemberUnrecoverable, volume: volumeWithData(statefulSet, existing, podName)}
		}
	}
	return nonMembers
}

// volumeWithData returns a volume of the given Pod of the given StatefulSet that may hold data, as "claim <name>" or
// "volume <name>", or an empty string if there is none. Any claim may hold data, whatever its state: it is not looked up.
// The StatefulSet controller creates the claims before the Pod and reuses the ones that exist, which may be bound to a
// volume that holds data, for example if they were retained after the Elasticsearch resource was deleted. A claim that
// the cache shows as missing or pending without a volume may have been created or bound since, and removing the Pod
// would then lose that data once the claim is garbage collected. A volume that is not a claim may hold data unless
// volumeHoldsNoData. So only a Pod that has no claim, for example one that has emptyDir volumes only, has none to name.
// The volumes are taken from the Pod if it exists, as it may run an older template than the StatefulSet's, and from the
// StatefulSet template otherwise.
func volumeWithData(statefulSet appsv1.StatefulSet, pod *corev1.Pod, podName string) string {
	volumes := statefulSet.Spec.Template.Spec.Volumes
	if pod != nil {
		volumes = pod.Spec.Volumes
	}
	claimNames := sets.New[string]()
	for _, claim := range statefulSet.Spec.VolumeClaimTemplates {
		claimNames.Insert(fmt.Sprintf("%s-%s", claim.Name, podName))
	}
	for _, volume := range volumes {
		switch {
		case volume.PersistentVolumeClaim != nil:
			claimNames.Insert(volume.PersistentVolumeClaim.ClaimName)
		case volume.Ephemeral != nil:
			// the claim of a generic ephemeral volume is named after its Pod and volume
			claimNames.Insert(fmt.Sprintf("%s-%s", podName, volume.Name))
		case !volumeHoldsNoData(volume):
			return "volume " + volume.Name
		}
	}
	if claimNames.Len() == 0 {
		return ""
	}
	// the claims of the claim templates are also volumes of an existing Pod, the set counts each one once: the first one,
	// by name, is reported
	return "claim " + sets.List(claimNames)[0]
}

// volumeHoldsNoData returns true if the given volume, which is not a claim, does not outlive its Pod or is read-only,
// including inline CSI and image volumes. Any other kind, for example a hostPath, is considered to hold data.
func volumeHoldsNoData(volume corev1.Volume) bool {
	return volume.EmptyDir != nil || volume.ConfigMap != nil || volume.Secret != nil ||
		volume.Projected != nil || volume.DownwardAPI != nil || volume.CSI != nil || volume.Image != nil
}

// isUnschedulable returns true if the given Pod is pending because it is not scheduled, either because the scheduler
// cannot place it or because a scheduling gate holds it, as opposed to a Pod that is pending while it starts. A Pod in
// any other phase, Failed or Succeeded included, is not: it is held as nonMemberRecoverable, as on main, as the
// StatefulSet controller is expected to replace it.
func isUnschedulable(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodPending {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled {
			return condition.Status == corev1.ConditionFalse &&
				(condition.Reason == corev1.PodReasonUnschedulable || condition.Reason == corev1.PodReasonSchedulingGated)
		}
	}
	return false
}

// clusterIsSettled returns true if the cluster is green with no shard activity, so that no node outside of it holds the
// only copy of any data. The health is fresh, not the memoized one, which may predate the nodes, and it must apply to
// the same nodes that membership was computed from: they are retrieved again after it and compared. Any retrieval
// failure means not settled.
func clusterIsSettled(ctx context.Context, esClient esclient.Client, esState ESState) bool {
	nodeNameToID, err := esState.NodeNameToID()
	if err != nil {
		ulog.FromContext(ctx).Error(err, "Cannot retrieve the nodes, holding the nodes that are not members of the cluster")
		return false
	}
	// wait for pending events, so that shard initializations are not missed
	health, err := esClient.GetClusterHealthWaitForAllEvents(ctx)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "Cannot retrieve the cluster health, holding the nodes that are not members of the cluster")
		return false
	}
	if health.NumberOfNodes != len(nodeNameToID) {
		ulog.FromContext(ctx).V(1).Info("The nodes of the cluster changed since they were retrieved, holding the nodes that are not members of the cluster",
			"health_nodes", health.NumberOfNodes, "nodes", len(nodeNameToID))
		return false
	}
	if health.Status != esv1.ElasticsearchGreenHealth || health.HasShardActivity() {
		return false
	}
	// the same count does not mean the same nodes: one may have joined while another left
	current, err := esClient.GetNodes(ctx)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "Cannot retrieve the nodes, holding the nodes that are not members of the cluster")
		return false
	}
	if current.IsPartial() || !maps.Equal(nodeNamesToIDs(current), nodeNameToID) {
		ulog.FromContext(ctx).V(1).Info("The nodes of the cluster changed since they were retrieved, holding the nodes that are not members of the cluster")
		return false
	}
	return true
}

// nodeNamesToIDs returns the IDs of the given nodes, by node name.
func nodeNamesToIDs(nodes esclient.Nodes) map[string]string {
	nodeNameToID := make(map[string]string, len(nodes.Nodes))
	for id, node := range nodes.Nodes {
		nodeNameToID[node.Name] = id
	}
	return nodeNameToID
}

// nodesNotInCluster returns the desired leaving nodes that are not members of the cluster. Without node shutdown,
// membership does not matter, as a node is removed once it holds no shard: none is returned.
func nodesNotInCluster(downscaleCtx downscaleContext, leaving *leavingNodes) (sets.Set[string], error) {
	if !supportsNodeShutdown(downscaleCtx.esClient.Version()) {
		return nil, nil
	}
	members, err := leaving.members(leaving.names())
	if err != nil {
		return nil, err
	}
	return leaving.desired.Difference(members), nil
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
	for _, node := range sets.List(withShutdown) {
		status, err := downscaleCtx.nodeShutdown.ShutdownStatus(downscaleCtx.parentCtx, node)
		if err != nil {
			return nil, fmt.Errorf("while checking shutdown status: %w", err)
		}
		statuses[node] = status
	}
	return statuses, nil
}

// reportShutdownStalled reports the first stalled shutdown, by priority then node name, through the phase and an event,
// as it can require user interaction. The shutdowns of the current statuses come first: a stalled one of them is
// reported rather than one of the unreached statuses, which come from a previous reconciliation, whatever the node names.
func reportShutdownStalled(downscaleCtx downscaleContext, current, unreached map[string]shutdown.NodeShutdownStatus) {
	for _, statuses := range []map[string]shutdown.NodeShutdownStatus{current, unreached} {
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
// StatefulSet name. It only needs the expected replicas, so desiredLeavingNodeNamesFromSpec can use it with the NodeSets.
func calculateDownscales(
	ctx context.Context,
	state downscaleState,
	expected expectedReplicas,
	actualStatefulSets es_sset.StatefulSetList,
	downscaleFilter downscaleFilter,
) (downscales []ssetDownscale, deletions es_sset.StatefulSetList, heldReasons map[string]string) {
	for _, actualSset := range actualStatefulSets {
		actualReplicas := sset.GetReplicas(actualSset)
		expectedCount, shouldExist := expected[actualSset.Name]

		switch {
		case !shouldExist && actualReplicas == 0 && expectedCount == 0:
			// the StatefulSet should not exist, and currently has no replicas
			// it is safe to delete
			deletions = append(deletions, actualSset)

		case expectedCount < actualReplicas:
			// the StatefulSet should be downscaled
			requestedDeletes := actualReplicas - expectedCount
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
			downscale := ssetDownscale{
				statefulSet:     actualSset,
				initialReplicas: actualReplicas,
				targetReplicas:  actualReplicas - allowedDeletes,
				finalReplicas:   expectedCount,
			}
			// the leaving non-members that have no shutdown to wait for
			downscale.removableNonMembers = state.removableNonMembersAmong(downscale.leavingNodeNames())
			downscales = append(downscales, downscale)

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
	if err := doDownscale(ctx, performable); err != nil {
		return shouldRequeue, err
	}
	// the removals without a shutdown are only reported once they are done, not on every attempt that fails
	for _, node := range performable.leavingNodeNames() {
		if nonMember, removable := downscale.removableNonMembers[node]; removable {
			reportNonMemberRemoval(ctx, downscale, node, nonMember)
		}
	}
	return shouldRequeue, nil
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

// reportNonMemberRemoval logs and records an event for the removal of the given non-member node without a shutdown. The
// event is a warning for a master: if it used to be a member, its vote may remain, as an exclusion by name is never
// resolved to the ID of an absent node, until Elasticsearch has at least 3 live masters to prefer over it. The volume
// that may hold data, if any, is named, as it may be deleted with the Pod.
func reportNonMemberRemoval(ctx downscaleContext, downscale ssetDownscale, node string, nonMember nonMember) {
	reason := nonMember.kind.removedReason
	if nonMember.volume != "" {
		reason += " (" + nonMember.volume + ")"
	}
	eventType := corev1.EventTypeNormal
	if label.IsMasterNodeSet(downscale.statefulSet) {
		reason += ". If it was a master that left the cluster, it may still be in the voting configuration until at " +
			"least 3 masters are live: with fewer, check the voting configuration"
		eventType = corev1.EventTypeWarning
	}
	// the log names the node in its own key, the event in its message
	ssetLogger(ctx.parentCtx, downscale.statefulSet).Info("Removed node without a shutdown: "+reason, "node", node)
	ctx.reconcileState.AddEvent(eventType, events.EventReasonRemoved, events.EventActionReconciliation,
		fmt.Sprintf("Removed node %s without a shutdown: %s", node, reason))
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
		statefulSet:         downscale.statefulSet,
		initialReplicas:     downscale.initialReplicas,
		targetReplicas:      downscale.initialReplicas, // target set to initial
		finalReplicas:       downscale.finalReplicas,
		removableNonMembers: downscale.removableNonMembers,
	}
	// iterate on all leaving nodes (ordered by highest ordinal first)
	for _, node := range downscale.leavingNodeNames() {
		if _, removable := downscale.removableNonMembers[node]; removable {
			// a node that is not a member of the cluster has no shutdown to wait for, its removal is reported once done
			performableDownscale.targetReplicas--
			continue
		}
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
