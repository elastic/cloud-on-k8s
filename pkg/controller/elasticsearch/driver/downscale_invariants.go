// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

const (
	OneMasterAtATimeInvariant        = "A master node is already in the process of being removed"
	AtLeastOneRunningMasterInvariant = "Cannot remove the last running master node"
)

// checkDownscaleInvariants returns true if the given state state allows downscaling the given StatefulSet.
// If not, it also returns the reason why.
func checkDownscaleInvariants(state downscaleState, statefulSet appsv1.StatefulSet) (bool, string) {
	if !label.IsMasterNodeSet(statefulSet) {
		// only care about master nodes
		return true, ""
	}
	if state.masterRemovalInProgress {
		return false, OneMasterAtATimeInvariant
	}
	if state.runningMasters == 1 {
		return false, AtLeastOneRunningMasterInvariant
	}
	return true, ""
}

// downscaleState tracks the state of a downscale to be checked against invariants
type downscaleState struct {
	// masterRemovalInProgress indicates whether a master node is in the process of being removed already.
	masterRemovalInProgress bool
	// runningMasters indicates how many masters are currently running in the cluster.
	runningMasters int
}

// newDownscaleState creates a new downscaleState.
func newDownscaleState(c k8s.Client, es v1alpha1.Elasticsearch) (*downscaleState, error) {
	// retrieve the number of masters running ready
	actualPods, err := sset.GetActualPodsForCluster(c, es)
	if err != nil {
		return nil, err
	}
	mastersReady := reconcile.AvailableElasticsearchNodes(label.FilterMasterNodePods(actualPods))

	return &downscaleState{
		masterRemovalInProgress: false,
		runningMasters:          len(mastersReady),
	}, nil
}

// recordOneRemoval updates the state to consider a 1-replica downscale of the given statefulSet.
func (s *downscaleState) recordOneRemoval(statefulSet appsv1.StatefulSet) {
	if !label.IsMasterNodeSet(statefulSet) {
		// only care about master nodes
		return
	}
	s.masterRemovalInProgress = true
	s.runningMasters--
}
