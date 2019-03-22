// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
)

func quorum(nMasters int) int {
	if nMasters == 0 {
		return 0
	}
	return (nMasters / 2) + 1
}

// ComputeMinimumMasterNodes returns the minimum number of master nodes
// that should be set in a cluster with the given topology
func ComputeMinimumMasterNodes(topology []v1alpha1.TopologyElementSpec) int {
	nMasters := 0
	for _, t := range topology {
		if t.NodeTypes.Master {
			nMasters += int(t.NodeCount)
		}
	}
	return quorum(nMasters)
}

// ComputeMinimumMasterNodesFromPods returns the minimum number of master nodes based on the
// current topology of the cluster.
func ComputeMinimumMasterNodesFromPods(cluster []corev1.Pod) int {
	nMasters := 0
	for _, p := range cluster {
		if label.IsMasterNode(p) {
			nMasters++
		}
	}
	return quorum(nMasters)
}
