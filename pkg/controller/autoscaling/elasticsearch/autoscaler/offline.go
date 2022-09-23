// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

// GetOfflineNodeSetsResources attempts to create or restore resources.NodeSetsResources without an actual autoscaling
// response from Elasticsearch. It ensures that even if no response has been returned by the autoscaling API then
// the NodeSets still respect the min. and max. resource requirements specified by the user.
// If resources are within the min. and max. boundaries then they are left untouched.
func GetOfflineNodeSetsResources(
	log logr.Logger,
	nodeSets []string,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus,
) v1alpha1.NodeSetsResources {
	currentNodeSetsResources, hasNodeSetsResources := currentAutoscalingStatus.CurrentResourcesForPolicy(autoscalingSpec.Name)

	var nodeSetsResources v1alpha1.NodeSetsResources
	var expectedNodeCount int32
	if !hasNodeSetsResources {
		// There's no current status for this nodeSet, this happens when the Elasticsearch cluster does not exist.
		// In that case we create a new one from the minimum values provided by the user.
		nodeSetsResources = newMinNodeSetResources(autoscalingSpec, nodeSets)
	} else {
		// The status contains some resource values for the NodeSets managed by this autoscaling policy, let's reuse them.
		nodeSetsResources = nodeSetResourcesFromStatus(currentNodeSetsResources, autoscalingSpec, nodeSets)
		for _, nodeSet := range currentNodeSetsResources.NodeSetNodeCount {
			expectedNodeCount += nodeSet.NodeCount
		}
	}

	// Ensure that the number of nodes is in the allowed range.
	expectedNodeCount = autoscalingSpec.NodeCountRange.Enforce(expectedNodeCount)

	// Adjust limits
	nodeSetsResources.NodeResources = nodeSetsResources.UpdateLimits(autoscalingSpec.AutoscalingResources)

	// User may have added or removed some NodeSets while the autoscaling API is not available.
	// We distribute the nodes to reflect that change.
	distributeFairly(nodeSetsResources.NodeSetNodeCount, expectedNodeCount)

	log.Info(
		"Offline autoscaling",
		"state", "offline",
		"policy", autoscalingSpec.Name,
		"nodeset", nodeSetsResources.NodeSetNodeCount,
		"count", nodeSetsResources.NodeSetNodeCount.TotalNodeCount(),
		"resources", nodeSetsResources.ToInt64(),
	)
	return nodeSetsResources
}

// nodeSetResourcesFromStatus restores NodeSetResources from the status.
// Resources are adjusted to respect min and max in the autoscaling specification, except for storage.
// If a resource is expected but not present in the status then the min. value in the autoscaling specification is used.
// If user removed the limits while offline we are assuming that they want to take back control of the resources.
func nodeSetResourcesFromStatus(
	currentNodeSetsResources v1alpha1.NodeSetsResources,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	nodeSets []string,
) v1alpha1.NodeSetsResources {
	nodeSetsResources := v1alpha1.NewNodeSetsResources(autoscalingSpec.Name, nodeSets)
	if autoscalingSpec.IsMemoryDefined() {
		// Attempt to get memory value from the status.
		if currentNodeSetsResources.HasRequest(corev1.ResourceMemory) {
			nodeSetsResources.SetRequest(
				corev1.ResourceMemory,
				autoscalingSpec.MemoryRange.Enforce(currentNodeSetsResources.GetRequest(corev1.ResourceMemory)),
			)
		} else {
			// Can't restore memory from status, use the min. from the autoscaling specification.
			nodeSetsResources.SetRequest(corev1.ResourceMemory, autoscalingSpec.MemoryRange.Min.DeepCopy())
		}
	}

	if autoscalingSpec.IsCPUDefined() {
		// Attempt to get CPU value from the status.
		if currentNodeSetsResources.HasRequest(corev1.ResourceCPU) {
			nodeSetsResources.SetRequest(
				corev1.ResourceCPU,
				autoscalingSpec.CPURange.Enforce(currentNodeSetsResources.GetRequest(corev1.ResourceCPU)),
			)
		} else {
			// Can't restore CPU from status, use the min. from the autoscaling specification.
			nodeSetsResources.SetRequest(corev1.ResourceCPU, autoscalingSpec.CPURange.Min.DeepCopy())
		}
	}

	if autoscalingSpec.IsStorageDefined() {
		storage := autoscalingSpec.StorageRange.Min
		// Attempt to get storage value from the status.
		if currentNodeSetsResources.HasRequest(corev1.ResourceStorage) {
			storageInStatus := currentNodeSetsResources.GetRequest(corev1.ResourceStorage)
			// Only use storage in status if it is greater than min value to respect user specification.
			// We do not adjust storage re. to the max value specified by the user since we don't want to decrease the storage capacity.
			if storageInStatus.Cmp(storage) > 0 {
				storage = storageInStatus
			}
		}
		nodeSetsResources.SetRequest(corev1.ResourceStorage, storage)
	}
	return nodeSetsResources
}

// newMinNodeSetResources returns a NodeSetResources with minimums values
func newMinNodeSetResources(autoscalingSpec v1alpha1.AutoscalingPolicySpec, nodeSets []string) v1alpha1.NodeSetsResources {
	nodeSetsResources := v1alpha1.NewNodeSetsResources(autoscalingSpec.Name, nodeSets)
	if autoscalingSpec.IsCPUDefined() {
		nodeSetsResources.SetRequest(corev1.ResourceCPU, autoscalingSpec.CPURange.Min.DeepCopy())
	}
	if autoscalingSpec.IsMemoryDefined() {
		nodeSetsResources.SetRequest(corev1.ResourceMemory, autoscalingSpec.MemoryRange.Min.DeepCopy())
	}
	if autoscalingSpec.IsStorageDefined() {
		nodeSetsResources.SetRequest(corev1.ResourceStorage, autoscalingSpec.StorageRange.Min.DeepCopy())
	}
	return nodeSetsResources
}
