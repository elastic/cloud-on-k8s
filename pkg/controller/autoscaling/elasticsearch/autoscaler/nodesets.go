// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	"sort"
	"strings"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

func distributeFairly(nodeSets v1alpha1.NodeSetNodeCountList, expectedNodeCount int32) {
	if len(nodeSets) == 0 {
		return
	}
	// sort the slice a first time
	sortNodeSets(nodeSets)
	for expectedNodeCount > 0 {
		// Peak the first element, this is the one with fewer nodes
		nodeSets[0].NodeCount++
		// Ensure the set is sorted
		sortNodeSets(nodeSets)
		expectedNodeCount--
	}
}

// sort sorts node sets by the value of the Count field, giving priority to node sets with fewer nodes.
// If several node sets have the same number of nodes they are sorted alphabetically.
func sortNodeSets(nodeSetNodeCountList v1alpha1.NodeSetNodeCountList) {
	sort.SliceStable(nodeSetNodeCountList, func(i, j int) bool {
		if nodeSetNodeCountList[i].NodeCount == nodeSetNodeCountList[j].NodeCount {
			return strings.Compare(nodeSetNodeCountList[i].Name, nodeSetNodeCountList[j].Name) < 0
		}
		return nodeSetNodeCountList[i].NodeCount < nodeSetNodeCountList[j].NodeCount
	})
}
