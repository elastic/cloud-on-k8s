// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// GetOfflineNodeSetsResources attempts to create or restore resources.NodeSetsResources without an actual autoscaling
// decision from Elasticsearch. It ensures that even if no decision has been returned by the autoscaling API then
// the NodeSets still respect the min. and max. resource requirements specified by the user.
// If resources are within the min. and max. boundaries then they are left untouched.
func GetOfflineNodeSetsResources(
	log logr.Logger,
	nodeSets []string,
	autoscalingSpec esv1.AutoscalingPolicySpec,
	actualAutoscalingStatus status.Status,
) resources.NodeSetsResources {
	actualNamedTierResources, hasNamedTierResources := actualAutoscalingStatus.GetNamedTierResources(autoscalingSpec.Name)

	var namedTierResources resources.NodeSetsResources
	var expectedNodeCount int32
	if !hasNamedTierResources {
		// There's no current status for this nodeSet, this happens when the Elasticsearch cluster does not exist.
		// In that case we create a new one from the minimum values provided by the user.
		namedTierResources = newMinNodeSetResources(autoscalingSpec, nodeSets)
	} else {
		// The status contains some resource values for the NodeSets managed by this autoscaling policy, let's reuse them.
		namedTierResources = nodeSetResourcesFromStatus(actualAutoscalingStatus, actualNamedTierResources, autoscalingSpec, nodeSets)
		for _, nodeSet := range actualNamedTierResources.NodeSetNodeCount {
			expectedNodeCount += nodeSet.NodeCount
		}
	}

	// Ensure that the min. number of nodes is in the allowed range.
	if expectedNodeCount < autoscalingSpec.NodeCount.Min {
		expectedNodeCount = autoscalingSpec.NodeCount.Min
	} else if expectedNodeCount > autoscalingSpec.NodeCount.Max {
		expectedNodeCount = autoscalingSpec.NodeCount.Max
	}

	// User may have added or removed some NodeSets while the autoscaling API is not available.
	// We distribute the nodes to reflect that change.
	fnm := NewFairNodesManager(log, namedTierResources.NodeSetNodeCount)
	for expectedNodeCount > 0 {
		fnm.AddNode()
		expectedNodeCount--
	}

	log.Info(
		"Offline autoscaling",
		"state", "offline",
		"policy", autoscalingSpec.Name,
		"nodeset", namedTierResources.NodeSetNodeCount,
		"count", namedTierResources.NodeSetNodeCount.TotalNodeCount(),
		"resources", namedTierResources.ToInt64(),
	)
	return namedTierResources
}

// nodeSetResourcesFromStatus restores NodeSetResources from the status.
// If user removed the limits while offline we are assuming that it wants to take back control on the resources.
func nodeSetResourcesFromStatus(
	actualAutoscalingStatus status.Status,
	actualNamedTierResources resources.NodeSetsResources,
	autoscalingSpec esv1.AutoscalingPolicySpec,
	nodeSets []string,
) resources.NodeSetsResources {
	namedTierResources := resources.NewNodeSetsResources(autoscalingSpec.Name, nodeSets)
	// Ensure memory settings are in the allowed limit range.
	if autoscalingSpec.IsMemoryDefined() {
		if actualNamedTierResources.HasRequest(corev1.ResourceMemory) {
			namedTierResources.SetRequest(
				corev1.ResourceMemory,
				adjustQuantity(actualNamedTierResources.GetRequest(corev1.ResourceMemory), autoscalingSpec.Memory.Min, autoscalingSpec.Memory.Max),
			)
		} else {
			namedTierResources.SetRequest(corev1.ResourceMemory, autoscalingSpec.Memory.Min.DeepCopy())
		}
	}

	// Ensure CPU settings are in the allowed limit range.
	if autoscalingSpec.IsCPUDefined() {
		if actualNamedTierResources.HasRequest(corev1.ResourceCPU) {
			namedTierResources.SetRequest(
				corev1.ResourceCPU,
				adjustQuantity(actualNamedTierResources.GetRequest(corev1.ResourceCPU), autoscalingSpec.CPU.Min, autoscalingSpec.CPU.Max),
			)
		} else {
			namedTierResources.SetRequest(corev1.ResourceCPU, autoscalingSpec.CPU.Min.DeepCopy())
		}
	}

	// Ensure storage capacity is set
	namedTierResources.SetRequest(corev1.ResourceStorage, getStorage(autoscalingSpec, actualAutoscalingStatus))
	return namedTierResources
}

// newMinNodeSetResources returns a NodeSetResources with minimums values
func newMinNodeSetResources(autoscalingSpec esv1.AutoscalingPolicySpec, nodeSets []string) resources.NodeSetsResources {
	namedTierResources := resources.NewNodeSetsResources(autoscalingSpec.Name, nodeSets)
	if autoscalingSpec.IsCPUDefined() {
		namedTierResources.SetRequest(corev1.ResourceCPU, autoscalingSpec.CPU.Min.DeepCopy())
	}
	if autoscalingSpec.IsMemoryDefined() {
		namedTierResources.SetRequest(corev1.ResourceMemory, autoscalingSpec.Memory.Min.DeepCopy())
	}
	if autoscalingSpec.IsStorageDefined() {
		namedTierResources.SetRequest(corev1.ResourceStorage, autoscalingSpec.Storage.Min.DeepCopy())
	}
	return namedTierResources
}

// adjustQuantity ensures that a quantity is comprised between a min and a max.
func adjustQuantity(value, min, max resource.Quantity) resource.Quantity {
	if value.Cmp(min) < 0 {
		return min
	} else if value.Cmp(max) > 0 {
		return max
	}
	return value
}
