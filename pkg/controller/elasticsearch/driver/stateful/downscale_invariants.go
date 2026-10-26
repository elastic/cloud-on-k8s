// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	OneMasterAtATimeInvariant        = "A master node is already in the process of being removed"
	AtLeastOneRunningMasterInvariant = "Cannot remove the last running master node"
	RespectMaxUnavailableInvariant   = "Not removing node to respect maxUnavailable setting"
	// NodeNotInClusterInvariant is used for a nonMemberRecoverable node.
	NodeNotInClusterInvariant = "Not removing node that is not a member of the cluster until it joins it; if it keeps " +
		"failing, adjust the NodeSet spec to resolve the failures"
	// NodeNotInClusterPartialInvariant is used instead of NodeNotInClusterInvariant if the nodes response is partial.
	NodeNotInClusterPartialInvariant = "Not removing node that is not a member of the cluster, or that did not answer the nodes request"
	// NodeNotInClusterNotSettledInvariant is used for a nonMemberUnrecoverable node while clusterIsSettled is false,
	// followed by its volume, if it has one that may hold data.
	NodeNotInClusterNotSettledInvariant = "Not removing node that is not a member of the cluster and cannot join it until the cluster is confirmed green with no shard activity"
)

// checkDownscaleInvariants returns the removals of nodes that the given state allows when downscaling the given
// StatefulSet. If fewer nodes than the requested deletes can be removed, it also returns the reason why.
func checkDownscaleInvariants(state downscaleState, statefulSet appsv1.StatefulSet, requestedDeletes int32) (nodeRemovals, string) {
	// mastersHeld indicates whether master removals are held by the one master at a time invariant
	var mastersHeld bool
	if label.IsMasterNodeSet(statefulSet) {
		if reason := checkMasterRemovalInvariants(state, statefulSet); reason != "" {
			return nodeRemovals{}, reason
		}
		if requestedDeletes > 1 {
			// only one removal allowed for masters, the other masters are held until it is over
			requestedDeletes = 1
			mastersHeld = true
		}
	}
	removals := state.getMaxNodesToRemove(statefulSet, requestedDeletes)

	if removals.count < requestedDeletes {
		// the removals stop at the next Pod: it may be a node that is not a member of the cluster, unless it is removable,
		// as then the budget holds it
		next := sset.PodName(statefulSet.Name, sset.GetReplicas(statefulSet)-removals.count-1)
		if state.isNotMember(next) && !state.isRemovableNonMember(next) {
			return removals, state.nonMemberHeldReason(next)
		}
	}

	switch {
	case removals.count == 0:
		return nodeRemovals{}, RespectMaxUnavailableInvariant
	case mastersHeld:
		return removals, OneMasterAtATimeInvariant
	case removals.count < requestedDeletes:
		return removals, RespectMaxUnavailableInvariant
	}

	return removals, ""
}

// checkMasterRemovalInvariants returns why the next master of the given master StatefulSet cannot be removed, or an
// empty string if it can.
func checkMasterRemovalInvariants(state downscaleState, statefulSet appsv1.StatefulSet) string {
	if state.masterRemovalInProgress {
		return OneMasterAtATimeInvariant
	}
	podName := sset.PodName(statefulSet.Name, sset.GetReplicas(statefulSet)-1)
	// the removal of another master is in progress if it is draining, unless this master is draining as well
	if len(state.drainingMasters) > 0 && !state.drainingMasters.Has(podName) {
		// name the draining masters, as their removal may be held without being visible otherwise
		return fmt.Sprintf("%s: %s draining", OneMasterAtATimeInvariant, strings.Join(sets.List(state.drainingMasters), ", "))
	}
	removedRunningMasters := 0
	if state.isRunningMaster(podName) {
		removedRunningMasters = 1
	}
	// at least one running master must remain after the removal
	if state.runningMasters-removedRunningMasters < 1 {
		return AtLeastOneRunningMasterInvariant
	}
	return ""
}

// downscaleState tracks the state of a downscale to be checked against invariants
type downscaleState struct {
	// runningMasters indicates how many masters are currently ready and not terminating in the cluster.
	runningMasters int
	// removalsAllowed indicates how many nodes can be removed to adhere to maxUnavailable setting,
	// nil indicates that any number of removals is allowed. Negative value is not expected.
	removalsAllowed *int32
	// masterRemovalInProgress indicates whether a master node is in the process of being removed already.
	masterRemovalInProgress bool
	// unavailableDrainingPods are the not ready, non-terminating Pods of the nodes the downscale is draining. Terminating
	// Pods may be stopping, so their removal conservatively consumes the budget.
	unavailableDrainingPods sets.Set[string]
	// unavailableMasters are the master Pods that are not ready, terminating or missing, they are not running masters.
	unavailableMasters sets.Set[string]
	// drainingRemovalsAllowed indicates how many draining nodes can be removed, so that they are not all removed at once,
	// nil indicates that any number of removals is allowed.
	drainingRemovalsAllowed *int32
	// drainingMasters are the draining master Pods that are the next to be removed from their StatefulSet: their removal
	// is in progress, even if they are ready or terminating, as their shutdown is kept.
	drainingMasters sets.Set[string]
	// readyPods are the ready Pods, which the budget counts as available. A Pod that is not a member of the cluster is
	// usually not ready, but may be, for example with a readiness probe that does not check membership.
	readyPods sets.Set[string]
	// nonMembers are the leaving Pods that are not members of the cluster, by name, see classifyNonMembers.
	nonMembers map[string]nonMember
	// nonMemberRemovalsAllowed returns true if the nonMemberUnrecoverable Pods can be removed, nil meaning they cannot.
	// It may request Elasticsearch, so it is only called when such a Pod is the next to be removed.
	nonMemberRemovalsAllowed func() bool
	// partialNodes is true if the nodes response is partial: a Pod that is not a member may be a member that did not
	// answer, which the reason why its removal is held says.
	partialNodes bool
}

// nonMemberKind tells what the downscale can do with a leaving node that is not a member of the cluster. The kinds are
// the variables below: each one holds all of its behavior, so a new kind cannot miss a case.
type nonMemberKind struct {
	// removable returns true if a node of this kind can be removed without a shutdown in the given state, nil meaning it
	// cannot.
	removable func(s *downscaleState) bool
	// heldReason is why the removal of a node of this kind is held, unless the nodes response is partial.
	heldReason string
	// removedReason is why a removable node of this kind is removed without a shutdown.
	removedReason string
}

var (
	// nonMemberRecoverable may still join the cluster: it is held until it does.
	nonMemberRecoverable = &nonMemberKind{
		heldReason: NodeNotInClusterInvariant,
	}
	// nonMemberUnrecoverable cannot join, and may have run with storage that holds data: a volume, a claim whatever its
	// state, or ephemeral volumes that survive container restarts. It is removed without a shutdown once the nodes response
	// is complete and the cluster is settled, as then every shard has all of its copies on the members, and its storage
	// can only hold stale ones, see nonMemberRemovalsAllowed. Whether the Pod ever ran is not used: only a Pod with no
	// storage that outlives it could skip the wait, which is rare enough not to be worth a kind of its own.
	nonMemberUnrecoverable = &nonMemberKind{
		removable:     removableOnceSettled,
		heldReason:    NodeNotInClusterNotSettledInvariant,
		removedReason: "it is not a member of the cluster and cannot join it, and the cluster is green with no shard activity: its storage holds no shard copy that the cluster lacks",
	}
)

// removableOnceSettled returns true if the nodes response is complete and the cluster is settled, see
// nonMemberRemovalsAllowed.
func removableOnceSettled(s *downscaleState) bool {
	return s.nonMemberRemovalsAllowed != nil && s.nonMemberRemovalsAllowed()
}

// nonMember is a leaving node that is not a member of the cluster.
type nonMember struct {
	kind *nonMemberKind
	// volume is the volume of the node that may hold data, if any.
	volume string
}

// isNotMember returns true if the given Pod is not a member of the cluster.
func (s *downscaleState) isNotMember(podName string) bool {
	_, ok := s.nonMembers[podName]
	return ok
}

// isRemovableNonMember returns true if the given Pod is not a member of the cluster and its removal is allowed without
// a shutdown.
func (s *downscaleState) isRemovableNonMember(podName string) bool {
	nonMember, ok := s.nonMembers[podName]
	return ok && nonMember.kind.removable != nil && nonMember.kind.removable(s)
}

// removableNonMembersAmong returns the given Pods that are removable non-members, by name, or nil if there are none.
func (s *downscaleState) removableNonMembersAmong(podNames []string) map[string]nonMember {
	var removable map[string]nonMember
	for _, podName := range podNames {
		if !s.isRemovableNonMember(podName) {
			continue
		}
		if removable == nil {
			removable = make(map[string]nonMember)
		}
		removable[podName] = s.nonMembers[podName]
	}
	return removable
}

// nonMemberHeldReason returns why the removal of the given Pod, which is not a member of the cluster, is held.
func (s *downscaleState) nonMemberHeldReason(podName string) string {
	if s.partialNodes {
		return fmt.Sprintf("%s: %s", NodeNotInClusterPartialInvariant, podName)
	}
	nonMember := s.nonMembers[podName]
	var reason strings.Builder
	fmt.Fprintf(&reason, "%s: %s", nonMember.kind.heldReason, podName)
	if nonMember.volume != "" {
		fmt.Fprintf(&reason, " (%s)", nonMember.volume)
	}
	return reason.String()
}

// newDownscaleState creates a new downscaleState. nodesWithShutdown are the desired leaving nodes that have a remove-type
// shutdown registered.
func newDownscaleState(
	actualPods []corev1.Pod,
	actualStatefulSets es_sset.StatefulSetList,
	es esv1.Elasticsearch,
	nodesWithShutdown sets.Set[string],
) *downscaleState {
	// retrieve the masters that are not running: a ready but terminating master may be stopping
	masters := label.FilterMasterNodePods(actualPods)
	unavailableMasters := unavailablePods(masters)
	runningMasters := len(masters) - len(unavailableMasters)
	// the master Pods that do not exist, for example because their creation is rejected, are not running either
	for _, podName := range missingMasterPods(actualStatefulSets, masters) {
		unavailableMasters.Insert(podName)
	}
	nodesReady := reconcile.AvailableElasticsearchNodes(actualPods)
	maxUnavailable := es.Spec.UpdateStrategy.ChangeBudget.GetMaxUnavailableOrDefault()

	return &downscaleState{
		masterRemovalInProgress: false,
		runningMasters:          runningMasters,
		removalsAllowed: calculateRemovalsAllowed(
			int32(len(nodesReady)), //nolint:gosec // G115: node count cannot realistically overflow int32
			es.Spec.NodeCount(),
			maxUnavailable),
		unavailableDrainingPods: unavailableDrainingPods(actualPods, nodesWithShutdown),
		unavailableMasters:      unavailableMasters,
		drainingRemovalsAllowed: calculateDrainingRemovalsAllowed(maxUnavailable),
		drainingMasters:         drainingMasterNodes(actualStatefulSets, nodesWithShutdown),
		readyPods:               sets.New(k8s.PodNames(nodesReady)...),
	}
}

// drainingMasterNodes returns the names of the Pods with the highest ordinal of the given master StatefulSets that are
// draining. A draining master that is not the next to be removed from its StatefulSet does not have its removal in
// progress, otherwise a shutdown registered on it would block every master removal.
func drainingMasterNodes(statefulSets es_sset.StatefulSetList, nodesWithShutdown sets.Set[string]) sets.Set[string] {
	masters := sets.New[string]()
	for _, statefulSet := range statefulSets {
		replicas := sset.GetReplicas(statefulSet)
		if replicas == 0 || !label.IsMasterNodeSet(statefulSet) {
			continue
		}
		if podName := sset.PodName(statefulSet.Name, replicas-1); nodesWithShutdown.Has(podName) {
			masters.Insert(podName)
		}
	}
	return masters
}

// missingMasterPods returns the names of the Pods of the given master StatefulSets that are not in the given master Pods.
func missingMasterPods(statefulSets es_sset.StatefulSetList, masters []corev1.Pod) []string {
	actual := sets.New(k8s.PodNames(masters)...)
	var missing []string
	for _, statefulSet := range statefulSets {
		if !label.IsMasterNodeSet(statefulSet) {
			continue
		}
		for _, podName := range sset.PodNames(statefulSet) {
			if !actual.Has(podName) {
				missing = append(missing, podName)
			}
		}
	}
	return missing
}

// unavailablePods returns the names of the given Pods that are not ready or terminating.
func unavailablePods(pods []corev1.Pod) sets.Set[string] {
	unavailable := sets.New[string]()
	for _, pod := range pods {
		if !k8s.IsPodReady(pod) || !pod.DeletionTimestamp.IsZero() {
			unavailable.Insert(pod.Name)
		}
	}
	return unavailable
}

// unavailableDrainingPods returns the names of the not ready and non-terminating Pods of the given nodes with a shutdown.
func unavailableDrainingPods(pods []corev1.Pod, nodesWithShutdown sets.Set[string]) sets.Set[string] {
	notReady := sets.New[string]()
	for _, pod := range pods {
		if nodesWithShutdown.Has(pod.Name) && !k8s.IsPodReady(pod) && pod.DeletionTimestamp.IsZero() {
			notReady.Insert(pod.Name)
		}
	}
	return notReady
}

func calculateRemovalsAllowed(nodesReady, desiredNodes int32, maxUnavailable *int32) *int32 {
	if maxUnavailable == nil {
		return nil
	}

	minAvailable := desiredNodes - *maxUnavailable
	removalsAllowed := max(nodesReady-minAvailable, 0)

	return &removalsAllowed
}

// calculateDrainingRemovalsAllowed returns how many draining nodes can be removed: maxUnavailable, but at least one so
// that draining nodes are removed when maxUnavailable is 0.
func calculateDrainingRemovalsAllowed(maxUnavailable *int32) *int32 {
	if maxUnavailable == nil {
		return nil
	}
	return new(max(*maxUnavailable, 1))
}

// isDrainingRemoval returns true if removing the given Pod consumes a draining removal rather than the budget: its
// remove-type shutdown closed its readiness port, so it is already unavailable. A draining master only does so if it is
// the only unavailable master, otherwise its removal waits for the budget and its shutdown is kept.
func (s *downscaleState) isDrainingRemoval(podName string) bool {
	if !s.unavailableDrainingPods.Has(podName) {
		return false
	}
	// A draining Pod is not ready, so it is a master Pod if it is one of the unavailable masters. The role of the Pod is
	// used rather than the one of its StatefulSet, which may already have a data-only template while the Pod still runs
	// as a master. The limit is 1, not 0, because the draining master is one of the unavailable masters. If another
	// master is unavailable or missing, the removal of the draining master consumes the budget.
	return !s.unavailableMasters.Has(podName) || len(s.unavailableMasters) == 1
}

// isRunningMaster returns true if the given master Pod is running: a not ready, terminating or missing master, for
// example a draining one, is not running.
func (s *downscaleState) isRunningMaster(podName string) bool {
	return !s.unavailableMasters.Has(podName)
}

// nodeRemovals are the removals of the Pods with the highest ordinals of a StatefulSet, and what they consume.
type nodeRemovals struct {
	// count is the number of Pods to remove.
	count int32
	// cost is the budget consumed by the removals.
	cost int32
	// drainingRemovals is the number of draining removals consumed by the removals.
	drainingRemovals int32
}

// getMaxNodesToRemove returns the removals, up to the given number, of the Pods with the highest ordinals of the given
// StatefulSet that the budget and the draining removals allow. They stop at a node that is not a member of the cluster,
// unless it is removable, which only consumes the budget if its Pod is ready.
func (s *downscaleState) getMaxNodesToRemove(statefulSet appsv1.StatefulSet, noMoreThan int32) nodeRemovals {
	replicas := sset.GetReplicas(statefulSet)
	var removals nodeRemovals
	for removals.count < noMoreThan {
		// consider the next Pod with the highest ordinal
		next := removals
		next.count++
		podName := sset.PodName(statefulSet.Name, replicas-next.count)
		switch {
		case s.isRemovableNonMember(podName):
			// the node has no shutdown to wait for: it only consumes the budget if its Pod is ready, as it is available then
			if s.readyPods.Has(podName) {
				next.cost++
			}
		case s.isNotMember(podName):
			// the node cannot be shut down, and the ones with a lower ordinal are removed after it
			return removals
		case s.isDrainingRemoval(podName):
			next.drainingRemovals++
		default:
			next.cost++
		}
		if !withinLimit(next.cost, s.removalsAllowed) || !withinLimit(next.drainingRemovals, s.drainingRemovalsAllowed) {
			break
		}
		removals = next
	}
	return removals
}

// withinLimit returns true if the given value does not exceed the given limit, nil indicating no limit.
func withinLimit(value int32, limit *int32) bool {
	return limit == nil || value <= *limit
}

// recordNodeRemoval updates the state to consider the given removals of the given statefulSet.
func (s *downscaleState) recordNodeRemoval(statefulSet appsv1.StatefulSet, removals nodeRemovals) {
	if removals.count == 0 {
		return
	}

	if label.IsMasterNodeSet(statefulSet) {
		s.masterRemovalInProgress = true
		// only one master is removed at a time
		if s.isRunningMaster(sset.PodName(statefulSet.Name, sset.GetReplicas(statefulSet)-1)) {
			s.runningMasters--
		}
	}

	if s.removalsAllowed != nil {
		*s.removalsAllowed -= removals.cost
	}
	if s.drainingRemovalsAllowed != nil {
		*s.drainingRemovalsAllowed -= removals.drainingRemovals
	}
}
