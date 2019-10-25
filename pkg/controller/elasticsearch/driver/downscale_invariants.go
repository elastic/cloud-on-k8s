// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

const (
	OneMasterAtATimeInvariant        = "A master node is already in the process of being removed"
	AtLeastOneRunningMasterInvariant = "Cannot remove the last running master node"
	RespectMaxUnavailableInvariant   = "Not removing node to respect maxUnavailable setting"
)

// checkDownscaleInvariants returns true if the given state state allows downscaling the given StatefulSet.
// If not, it also returns the reason why.
func checkDownscaleInvariants(state downscaleState, statefulSet appsv1.StatefulSet) (bool, string) {
	// if removalsAllowed is nil then removals shouldn't be bounded here
	if state.removalsAllowed != nil && *state.removalsAllowed < 1 {
		return false, RespectMaxUnavailableInvariant
	}

	if label.IsMasterNodeSet(statefulSet) {
		if state.masterRemovalInProgress {
			return false, OneMasterAtATimeInvariant
		}
		if state.runningMasters == 1 {
			return false, AtLeastOneRunningMasterInvariant
		}
	}

	return true, ""
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
}

// newDownscaleState creates a new downscaleState.
func newDownscaleState(c k8s.Client, es v1beta1.Elasticsearch) (*downscaleState, error) {
	// retrieve the number of masters running ready
	actualPods, err := sset.GetActualPodsForCluster(c, es)
	if err != nil {
		return nil, err
	}
	mastersReady := reconcile.AvailableElasticsearchNodes(label.FilterMasterNodePods(actualPods))
	nodesReady := reconcile.AvailableElasticsearchNodes(actualPods)

	return &downscaleState{
		masterRemovalInProgress: false,
		runningMasters:          len(mastersReady),
		removalsAllowed: calculateRemovalsAllowed(
			int32(len(nodesReady)),
			es.Spec.NodeCount(),
			es.Spec.UpdateStrategy.ChangeBudget.GetMaxUnavailableOrDefault()),
	}, nil
}

func calculateRemovalsAllowed(nodesReady, desiredNodes int32, maxUnavailable *int32) *int32 {
	if maxUnavailable == nil {
		return nil
	}

	minAvailable := desiredNodes - *maxUnavailable
	removalsAllowed := nodesReady - minAvailable
	if removalsAllowed < 0 {
		removalsAllowed = 0
	}

	return &removalsAllowed
}

func (s *downscaleState) getMaxNodesToRemove(noMoreThan int32) int32 {
	if s.removalsAllowed == nil {
		return noMoreThan
	}

	if noMoreThan > *s.removalsAllowed {
		return *s.removalsAllowed
	}
	return noMoreThan
}

// recordNodeRemoval updates the state to consider n-replica downscale of the given statefulSet.
func (s *downscaleState) recordNodeRemoval(statefulSet appsv1.StatefulSet, accountedRemovals int32) {
	if accountedRemovals == 0 {
		return
	}

	if label.IsMasterNodeSet(statefulSet) {
		s.masterRemovalInProgress = true
		s.runningMasters--
	}

	if s.removalsAllowed != nil {
		*s.removalsAllowed -= accountedRemovals
	}
}
