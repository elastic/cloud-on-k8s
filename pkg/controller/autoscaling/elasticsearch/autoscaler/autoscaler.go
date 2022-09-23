// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

// GetResources calculates the resources required by all the NodeSets managed by a same autoscaling policy.
func (ctx *Context) GetResources() v1alpha1.NodeSetsResources {
	// 1. Scale vertically, calculating the resources for each node managed by the autoscaling policy in the context.
	desiredNodeResources := ctx.scaleVertically()
	ctx.Log.Info(
		"Vertical autoscaler",
		"state", "online",
		"policy", ctx.AutoscalingSpec.Name,
		"scope", "node",
		"nodesets", ctx.NodeSets.Names(),
		"resources", desiredNodeResources.ToInt64(),
		"required_capacity", ctx.AutoscalingPolicyResult.RequiredCapacity,
	)

	// 2. Scale horizontally by adding nodes to meet the resource requirements.
	return ctx.stabilize(ctx.scaleHorizontally(desiredNodeResources))
}

// scaleVertically calculates the desired resources for all the nodes managed by the same autoscaling policy, given the requested
// capacity returned by the Elasticsearch autoscaling API and the AutoscalingSpec specified by the user.
// It attempts to scale all the resources vertically until the required resources are provided or the limits set by the user are reached.
func (ctx *Context) scaleVertically() v1alpha1.NodeResources {
	nodeResources := v1alpha1.NodeResources{}

	// Apply recommenders to get recommended quantities for each resource.
	for _, recommender := range ctx.Recommenders {
		if recommender.HasResourceRecommendation() {
			nodeResources.SetRequest(
				recommender.ManagedResource(),
				recommender.NodeResourceQuantity(),
			)
		}
	}

	// If no memory has been returned by the autoscaling API, but the user has expressed the intent to manage memory
	// using the autoscaling specification then we derive the memory from the storage if available.
	// See https://github.com/elastic/cloud-on-k8s/issues/4076
	if !nodeResources.HasRequest(corev1.ResourceMemory) && ctx.AutoscalingSpec.IsMemoryDefined() &&
		ctx.AutoscalingSpec.IsStorageDefined() && nodeResources.HasRequest(corev1.ResourceStorage) {
		nodeResources.SetRequest(corev1.ResourceMemory, memoryFromStorage(nodeResources.GetRequest(corev1.ResourceStorage), *ctx.AutoscalingSpec.StorageRange, *ctx.AutoscalingSpec.MemoryRange))
	}

	// Same as above, if CPU limits have been expressed by the user in the autoscaling specification then we adjust CPU request according to the memory request.
	// See https://github.com/elastic/cloud-on-k8s/issues/4021
	if !nodeResources.HasRequest(corev1.ResourceCPU) && ctx.AutoscalingSpec.IsCPUDefined() &&
		ctx.AutoscalingSpec.IsMemoryDefined() && nodeResources.HasRequest(corev1.ResourceMemory) {
		nodeResources.SetRequest(corev1.ResourceCPU, cpuFromMemory(nodeResources.GetRequest(corev1.ResourceMemory), *ctx.AutoscalingSpec.MemoryRange, *ctx.AutoscalingSpec.CPURange))
	}

	return nodeResources.UpdateLimits(ctx.AutoscalingSpec.AutoscalingResources)
}

// stabilize filters scale down decisions for a policy if the number of nodes observed by Elasticsearch is less than the expected one.
func (ctx *Context) stabilize(calculatedResources v1alpha1.NodeSetsResources) v1alpha1.NodeSetsResources {
	currentResources, hasCurrentResources := ctx.CurrentAutoscalingStatus.CurrentResourcesForPolicy(ctx.AutoscalingSpec.Name)
	if !hasCurrentResources {
		// Autoscaling policy does not have any resource yet, for example it might happen if it is a new tier.
		return calculatedResources
	}

	// currentNodeCount is a the number of nodes as previously stored in the status
	currentNodeCount := currentResources.NodeSetNodeCount.TotalNodeCount()
	// nextNodeCount is a the number of nodes as calculated in this reconciliation loop by the autoscaling algorithm
	nextNodeCount := calculatedResources.NodeSetNodeCount.TotalNodeCount()
	// scalingDown is the situation where the next number of nodes is less than the one in the status
	scalingDown := nextNodeCount < currentNodeCount
	// observedNodesByEs is the number of nodes used by Elasticsearch to compute its requirements
	observedNodesByEs := len(ctx.AutoscalingPolicyResult.CurrentNodes)
	//nolint:nestif
	if observedNodesByEs < int(currentNodeCount) && scalingDown {
		ctx.Log.Info(
			"Number of nodes observed by Elasticsearch is less than expected, do not scale down",
			"policy", ctx.AutoscalingSpec.Name,
			"observed.count", observedNodesByEs,
			"current.count", currentNodeCount,
			"next.count", nextNodeCount,
		)
		// The number of nodes observed by Elasticsearch is less than the expected one, do not scale down, reuse previous resources.
		// nextNodeSetNodeCountList is a copy of currentResources but resources are adjusted to respect limits set by the user in the spec.
		nextNodeSetNodeCountList := make(v1alpha1.NodeSetNodeCountList, len(currentResources.NodeSetNodeCount))
		for i := range currentResources.NodeSetNodeCount {
			nextNodeSetNodeCountList[i] = v1alpha1.NodeSetNodeCount{Name: currentResources.NodeSetNodeCount[i].Name}
		}
		distributeFairly(nextNodeSetNodeCountList, ctx.AutoscalingSpec.NodeCountRange.Enforce(currentNodeCount))
		nextResources := v1alpha1.NodeSetsResources{
			Name:             currentResources.Name,
			NodeSetNodeCount: nextNodeSetNodeCountList,
			NodeResources: v1alpha1.NodeResources{
				Requests: currentResources.Requests.DeepCopy(),
			},
		}
		// Reuse and adjust memory
		if ctx.AutoscalingSpec.IsMemoryDefined() && currentResources.HasRequest(corev1.ResourceMemory) {
			nextResources.SetRequest(corev1.ResourceMemory, ctx.AutoscalingSpec.MemoryRange.Enforce(currentResources.GetRequest(corev1.ResourceMemory)))
		}
		// Reuse and adjust CPU
		if ctx.AutoscalingSpec.IsCPUDefined() && currentResources.HasRequest(corev1.ResourceCPU) {
			nextResources.SetRequest(corev1.ResourceCPU, ctx.AutoscalingSpec.CPURange.Enforce(currentResources.GetRequest(corev1.ResourceCPU)))
		}
		// Reuse and adjust storage
		if ctx.AutoscalingSpec.IsStorageDefined() && currentResources.HasRequest(corev1.ResourceStorage) {
			storage := currentResources.GetRequest(corev1.ResourceStorage)
			// For storage we only ensure that we are greater than the min. value.
			if storage.Cmp(ctx.AutoscalingSpec.StorageRange.Min) < 0 {
				storage = ctx.AutoscalingSpec.StorageRange.Min.DeepCopy()
			}
			nextResources.SetRequest(corev1.ResourceStorage, storage)
		}

		// Also update and adjust limits if user has updated the ratios
		nextResources.NodeResources = nextResources.UpdateLimits(ctx.AutoscalingSpec.AutoscalingResources)

		return nextResources
	}
	return calculatedResources
}

// scaleHorizontally adds or removes nodes in a set of node sets to provide the required capacity in a tier.
func (ctx *Context) scaleHorizontally(
	nodeCapacity v1alpha1.NodeResources, // resources for each node in the tier/policy, as computed by the vertical autoscaler.
) v1alpha1.NodeSetsResources {
	var nodeCount int32
	for _, recommender := range ctx.Recommenders {
		if recommender.HasResourceRecommendation() {
			nodeCount = max(nodeCount, recommender.NodeCount(nodeCapacity))
		}
	}

	ctx.Log.Info("Horizontal autoscaler", "policy", ctx.AutoscalingSpec.Name,
		"scope", "tier",
		"count", nodeCount,
		"required_capacity", ctx.AutoscalingPolicyResult.RequiredCapacity.Total,
	)

	nodeSetsResources := v1alpha1.NewNodeSetsResources(ctx.AutoscalingSpec.Name, ctx.NodeSets.Names())
	nodeSetsResources.NodeResources = nodeCapacity
	distributeFairly(nodeSetsResources.NodeSetNodeCount, nodeCount)

	return nodeSetsResources
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
