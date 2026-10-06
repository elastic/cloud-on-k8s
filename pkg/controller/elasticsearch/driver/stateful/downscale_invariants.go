// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
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

// checkDownscaleInvariants returns the number of nodes that can be removed if the given state state allows downscaling
// the given StatefulSet. If that number is 0, it also returns the reason why.
func checkDownscaleInvariants(state downscaleState, statefulSet appsv1.StatefulSet, requestedDeletes int32) (int32, string) {
	if label.IsMasterNodeSet(statefulSet) {
		if state.masterRemovalInProgress {
			return 0, OneMasterAtATimeInvariant
		}
		requestedDeletes = 1 // only one removal allowed for masters
		podName := sset.PodName(statefulSet.Name, sset.GetReplicas(statefulSet)-1)
		// the removal of another master is in progress if it is draining, unless this master is draining as well
		if len(state.drainingMasters) > 0 && !state.drainingMasters.Has(podName) {
			return 0, OneMasterAtATimeInvariant
		}
		// a draining master is not running, removing it does not remove the last running master
		if cost, _ := state.removalCost(statefulSet, requestedDeletes); state.runningMasters == 1 && cost == 1 {
			return 0, AtLeastOneRunningMasterInvariant
		}
	}
	allowedDeletes := state.getMaxNodesToRemove(statefulSet, requestedDeletes)

	if allowedDeletes == 0 {
		return 0, RespectMaxUnavailableInvariant
	}

	return allowedDeletes, ""
}

// downscaleState tracks the state of a downscale to be checked against invariants
type downscaleState struct {
	// runningMasters indicates how many masters are currently running in the cluster.
	runningMasters int
	// removalsAllowed indicates how many nodes can be removed to adhere to maxUnavailable setting,
	// nil indicates that any number of removals is allowed. Negative value is not expected.
	removalsAllowed *int32
	// masterRemovalInProgress indicates whether a master node is in the process of being removed already.
	masterRemovalInProgress bool
	// drainingNodes are the not ready, non-terminating Pods of the nodes the downscale is draining. Their remove-type
	// shutdown closed their readiness port, so removing them does not consume the budget. Terminating Pods may be
	// stopping and are conservatively accounted as running nodes.
	drainingNodes set.StringSet
	// drainingRemovalsAllowed indicates how many draining nodes can be removed, so that they are not all removed at once,
	// nil indicates that any number of removals is allowed.
	drainingRemovalsAllowed *int32
	// drainingMasters are the draining master Pods that are the next to be removed from their StatefulSet: their removal
	// is in progress, even if they are ready or terminating, as the shutdown of a terminating Pod is kept.
	drainingMasters set.StringSet
}

// newDownscaleState creates a new downscaleState.
func newDownscaleState(
	actualPods []corev1.Pod,
	actualStatefulSets es_sset.StatefulSetList,
	es esv1.Elasticsearch,
	drainingNodes set.StringSet,
) *downscaleState {
	// retrieve the number of masters running ready
	mastersReady := reconcile.AvailableElasticsearchNodes(label.FilterMasterNodePods(actualPods))
	nodesReady := reconcile.AvailableElasticsearchNodes(actualPods)
	maxUnavailable := es.Spec.UpdateStrategy.ChangeBudget.GetMaxUnavailableOrDefault()

	return &downscaleState{
		masterRemovalInProgress: false,
		runningMasters:          len(mastersReady),
		removalsAllowed: calculateRemovalsAllowed(
			int32(len(nodesReady)), //nolint:gosec // G115: node count cannot realistically overflow int32
			es.Spec.NodeCount(),
			maxUnavailable),
		drainingNodes:           notReadyDrainingNodes(actualPods, drainingNodes),
		drainingRemovalsAllowed: calculateDrainingRemovalsAllowed(maxUnavailable),
		drainingMasters:         drainingMasterNodes(actualStatefulSets, drainingNodes),
	}
}

// drainingMasterNodes returns the names of the Pods with the highest ordinal of the given master StatefulSets that are
// draining. A draining master that is not the next to be removed from its StatefulSet does not have its removal in
// progress, otherwise a shutdown registered on it would block every master removal.
func drainingMasterNodes(statefulSets es_sset.StatefulSetList, drainingNodes set.StringSet) set.StringSet {
	var masters set.StringSet
	for _, statefulSet := range statefulSets {
		replicas := sset.GetReplicas(statefulSet)
		if replicas == 0 || !label.IsMasterNodeSet(statefulSet) {
			continue
		}
		podName := sset.PodName(statefulSet.Name, replicas-1)
		if !drainingNodes.Has(podName) {
			continue
		}
		if masters == nil {
			masters = set.Make()
		}
		masters.Add(podName)
	}
	return masters
}

// notReadyDrainingNodes returns the names of the not ready and non-terminating Pods of the given draining nodes.
func notReadyDrainingNodes(pods []corev1.Pod, drainingNodes set.StringSet) set.StringSet {
	var notReady set.StringSet
	for _, pod := range pods {
		if !drainingNodes.Has(pod.Name) || k8s.IsPodReady(pod) || !pod.DeletionTimestamp.IsZero() {
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

// removalCost returns the budget and the draining removals consumed by removing the given number of Pods with the
// highest ordinals of the given StatefulSet: the Pods of draining nodes are already unavailable and only consume a
// draining removal.
func (s *downscaleState) removalCost(statefulSet appsv1.StatefulSet, removals int32) (cost int32, drainingRemovals int32) {
	replicas := sset.GetReplicas(statefulSet)
	for ordinal := replicas - removals; ordinal < replicas; ordinal++ {
		if s.drainingNodes.Has(sset.PodName(statefulSet.Name, ordinal)) {
			drainingRemovals++
		} else {
			cost++
		}
	}
	return cost, drainingRemovals
}

func (s *downscaleState) getMaxNodesToRemove(statefulSet appsv1.StatefulSet, noMoreThan int32) int32 {
	replicas := sset.GetReplicas(statefulSet)
	var removals, cost, drainingRemovals int32
	for removals < noMoreThan {
		// consider the next Pod with the highest ordinal
		podName := sset.PodName(statefulSet.Name, replicas-1-removals)
		if s.drainingNodes.Has(podName) {
			drainingRemovals++
		} else {
			cost++
		}
		if !withinLimit(cost, s.removalsAllowed) || !withinLimit(drainingRemovals, s.drainingRemovalsAllowed) {
			break
		}
		removals++
	}
	return removals
}

// withinLimit returns true if the given value does not exceed the given limit, nil indicating no limit.
func withinLimit(value int32, limit *int32) bool {
	return limit == nil || value <= *limit
}

// recordNodeRemoval updates the state to consider n-replica downscale of the given statefulSet.
func (s *downscaleState) recordNodeRemoval(statefulSet appsv1.StatefulSet, accountedRemovals int32) {
	if accountedRemovals == 0 {
		return
	}

	cost, drainingRemovals := s.removalCost(statefulSet, accountedRemovals)
	if label.IsMasterNodeSet(statefulSet) {
		s.masterRemovalInProgress = true
		s.runningMasters -= int(cost)
	}

	if s.removalsAllowed != nil {
		*s.removalsAllowed -= cost
	}
	if s.drainingRemovalsAllowed != nil {
		*s.drainingRemovalsAllowed -= drainingRemovals
	}
}
