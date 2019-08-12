// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
)

const (
	OneMasterAtATimeInvariant        = "A master node is already in the process of being removed"
	AtLeastOneRunningMasterInvariant = "Cannot remove the last running master node"
)

// DownscaleInvariants restricts downscales to perform in a single reconciliation attempt:
// - remove a single master at once
// - don't remove the last living master node
type DownscaleInvariants struct {
	// masterRemoved indicates whether a master node is in the process of being removed already.
	masterRemoved bool
	// runningMasters indicates how many masters are currently running in the cluster.
	runningMasters int
}

// NewDownscaleInvariants creates a new DownscaleInvariants.
func NewDownscaleInvariants(c k8s.Client, es v1alpha1.Elasticsearch) (*DownscaleInvariants, error) {
	// retrieve the number of masters running ready
	actualPods, err := sset.GetActualPodsForCluster(c, es)
	if err != nil {
		return nil, err
	}
	mastersReady := reconcile.AvailableElasticsearchNodes(label.FilterMasterNodePods(actualPods))

	return &DownscaleInvariants{
		masterRemoved:  false,
		runningMasters: len(mastersReady),
	}, nil
}

// canDownscale returns true if the current state allows downscaling the given StatefulSet.
// If not, it also returns the reason why.
func (d *DownscaleInvariants) canDownscale(statefulSet appsv1.StatefulSet) (bool, string) {
	if !label.IsMasterNodeSet(statefulSet) {
		// only care about master nodes
		return true, ""
	}
	if d.masterRemoved {
		return false, OneMasterAtATimeInvariant
	}
	if d.runningMasters == 1 {
		return false, AtLeastOneRunningMasterInvariant
	}
	return true, ""
}

// accountDownscale updates the current invariants state to consider a 1-replica downscale of the given statefulSet.
func (d *DownscaleInvariants) accountOneRemoval(statefulSet appsv1.StatefulSet) {
	if !label.IsMasterNodeSet(statefulSet) {
		// only care about master nodes
		return
	}
	d.masterRemoved = true
	d.runningMasters--
}
