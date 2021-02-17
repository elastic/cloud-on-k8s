// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// GetResources calculates the resources required by all the NodeSets managed by a same autoscaling policy.
func (ctx *Context) GetResources() resources.NodeSetsResources {
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
func (ctx *Context) scaleVertically() resources.NodeResources {
	// All resources can be computed "from scratch", without knowing the previous values.
	// This is however not true for storage. Storage can't be scaled down, current storage capacity must be considered
	// as a hard min. limit. This storage limit must be taken into consideration when computing the desired resources.
	minStorage := getMinStorageQuantity(ctx.AutoscalingSpec, ctx.CurrentAutoscalingStatus)
	return ctx.nodeResources(
		int64(ctx.AutoscalingSpec.NodeCount.Min),
		minStorage,
	)
}

// getMinStorageQuantity returns the min. storage quantity that should be used by the autoscaling algorithm.
// The value is the max. value of either:
// * the current value in the status
// * the min. value set by the user in the autoscaling spec.
func getMinStorageQuantity(autoscalingSpec esv1.AutoscalingPolicySpec, currentAutoscalingStatus status.Status) resource.Quantity {
	// If no storage spec is defined in the autoscaling status we return the default volume size.
	storage := volume.DefaultPersistentVolumeSize.DeepCopy()
	// Always adjust to the min value specified by the user in the limits.
	if autoscalingSpec.IsStorageDefined() {
		storage = autoscalingSpec.Storage.Min
	}
	// If a storage value is stored in the status then reuse it.
	if currentResourcesInStatus, exists := currentAutoscalingStatus.CurrentResourcesForPolicy(autoscalingSpec.Name); exists && currentResourcesInStatus.HasRequest(corev1.ResourceStorage) {
		storageInStatus := currentResourcesInStatus.GetRequest(corev1.ResourceStorage)
		if storageInStatus.Cmp(storage) > 0 {
			storage = storageInStatus
		}
	}
	return storage
}

// stabilize filters scale down decisions for a policy if the number of nodes observed by Elasticsearch is less than the expected one.
func (ctx *Context) stabilize(calculatedResources resources.NodeSetsResources) resources.NodeSetsResources {
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
		nextNodeSetNodeCountList := make(resources.NodeSetNodeCountList, len(currentResources.NodeSetNodeCount))
		for i := range currentResources.NodeSetNodeCount {
			nextNodeSetNodeCountList[i] = resources.NodeSetNodeCount{Name: currentResources.NodeSetNodeCount[i].Name}
		}
		distributeFairly(nextNodeSetNodeCountList, ctx.AutoscalingSpec.NodeCount.Adjust(currentNodeCount))
		nextResources := resources.NodeSetsResources{
			Name:             currentResources.Name,
			NodeSetNodeCount: nextNodeSetNodeCountList,
			NodeResources: resources.NodeResources{
				Requests: currentResources.Requests.DeepCopy(),
			},
		}
		// Reuse and adjust memory
		if ctx.AutoscalingSpec.IsMemoryDefined() && currentResources.HasRequest(corev1.ResourceMemory) {
			nextResources.SetRequest(corev1.ResourceMemory, ctx.AutoscalingSpec.Memory.Adjust(currentResources.GetRequest(corev1.ResourceMemory)))
		}
		// Reuse and adjust CPU
		if ctx.AutoscalingSpec.IsCPUDefined() && currentResources.HasRequest(corev1.ResourceCPU) {
			nextResources.SetRequest(corev1.ResourceCPU, ctx.AutoscalingSpec.CPU.Adjust(currentResources.GetRequest(corev1.ResourceCPU)))
		}
		// Reuse and adjust storage
		if ctx.AutoscalingSpec.IsStorageDefined() && currentResources.HasRequest(corev1.ResourceStorage) {
			storage := currentResources.GetRequest(corev1.ResourceStorage)
			// For storage we only ensure that we are greater than the min. value.
			if storage.Cmp(ctx.AutoscalingSpec.Storage.Min) < 0 {
				storage = ctx.AutoscalingSpec.Storage.Min.DeepCopy()
			}
			nextResources.SetRequest(corev1.ResourceStorage, storage)
		}
		return nextResources

	}
	return calculatedResources
}
