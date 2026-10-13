// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

const (
	OneMasterAtATimeInvariant        = "A master node is already in the process of being removed"
	AtLeastOneRunningMasterInvariant = "Cannot remove the last running master node"
	RespectMaxUnavailableInvariant   = "Not removing node to respect maxUnavailable setting"
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
		return fmt.Sprintf("%s: %s draining", OneMasterAtATimeInvariant, strings.Join(state.drainingMasters.AsSortedSlice(), ", "))
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
	unavailableDrainingPods set.StringSet
	// unavailableMasters are the master Pods that are not ready, terminating or missing, they are not running masters.
	unavailableMasters set.StringSet
	// drainingRemovalsAllowed indicates how many draining nodes can be removed, so that they are not all removed at once,
	// nil indicates that any number of removals is allowed.
	drainingRemovalsAllowed *int32
	// drainingMasters are the draining master Pods that are the next to be removed from their StatefulSet: their removal
	// is in progress, even if they are ready or terminating, as their shutdown is kept.
	drainingMasters set.StringSet
}

// newDownscaleState creates a new downscaleState. nodesWithShutdown are the desired leaving nodes that have a remove-type
// shutdown registered.
func newDownscaleState(
	actualPods []corev1.Pod,
	actualStatefulSets es_sset.StatefulSetList,
	es esv1.Elasticsearch,
	nodesWithShutdown set.StringSet,
) *downscaleState {
	// retrieve the masters that are not running: a ready but terminating master may be stopping
	masters := label.FilterMasterNodePods(actualPods)
	unavailableMasters := unavailablePods(masters)
	runningMasters := len(masters) - len(unavailableMasters)
	// the master Pods that do not exist, for example because their creation is rejected, are not running either
	for _, podName := range missingMasterPods(actualStatefulSets, masters) {
		if unavailableMasters == nil {
			unavailableMasters = set.Make()
		}
		unavailableMasters.Add(podName)
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
	}
}

// drainingMasterNodes returns the names of the Pods with the highest ordinal of the given master StatefulSets that are
// draining. A draining master that is not the next to be removed from its StatefulSet does not have its removal in
// progress, otherwise a shutdown registered on it would block every master removal.
func drainingMasterNodes(statefulSets es_sset.StatefulSetList, nodesWithShutdown set.StringSet) set.StringSet {
	var masters set.StringSet
	for _, statefulSet := range statefulSets {
		replicas := sset.GetReplicas(statefulSet)
		if replicas == 0 || !label.IsMasterNodeSet(statefulSet) {
			continue
		}
		podName := sset.PodName(statefulSet.Name, replicas-1)
		if !nodesWithShutdown.Has(podName) {
			continue
		}
		if masters == nil {
			masters = set.Make()
		}
		masters.Add(podName)
	}
	return masters
}

// missingMasterPods returns the names of the Pods of the given master StatefulSets that are not in the given master Pods.
func missingMasterPods(statefulSets es_sset.StatefulSetList, masters []corev1.Pod) []string {
	actual := set.Make(k8s.PodNames(masters)...)
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
func unavailablePods(pods []corev1.Pod) set.StringSet {
	var unavailable set.StringSet
	for _, pod := range pods {
		if k8s.IsPodReady(pod) && pod.DeletionTimestamp.IsZero() {
			continue
		}
		if unavailable == nil {
			unavailable = set.Make()
		}
		unavailable.Add(pod.Name)
	}
	return unavailable
}

// unavailableDrainingPods returns the names of the not ready and non-terminating Pods of the given nodes with a shutdown.
func unavailableDrainingPods(pods []corev1.Pod, nodesWithShutdown set.StringSet) set.StringSet {
	var notReady set.StringSet
	for _, pod := range pods {
		if !nodesWithShutdown.Has(pod.Name) || k8s.IsPodReady(pod) || !pod.DeletionTimestamp.IsZero() {
			continue
		}
		if notReady == nil {
			notReady = set.Make()
		}
		notReady.Add(pod.Name)
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
func (s *downscaleState) isDrainingRemoval(statefulSet appsv1.StatefulSet, podName string) bool {
	if !s.unavailableDrainingPods.Has(podName) {
		return false
	}
	// the limit is 1, not 0, because the draining master is not ready and is one of the unavailable masters. If another
	// master is unavailable or missing, the removal of the draining master consumes the budget.
	return !label.IsMasterNodeSet(statefulSet) || len(s.unavailableMasters) == 1
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
// StatefulSet that the budget and the draining removals allow.
func (s *downscaleState) getMaxNodesToRemove(statefulSet appsv1.StatefulSet, noMoreThan int32) nodeRemovals {
	replicas := sset.GetReplicas(statefulSet)
	var removals nodeRemovals
	for removals.count < noMoreThan {
		// consider the next Pod with the highest ordinal
		next := removals
		next.count++
		if s.isDrainingRemoval(statefulSet, sset.PodName(statefulSet.Name, replicas-next.count)) {
			next.drainingRemovals++
		} else {
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
