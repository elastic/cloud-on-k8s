// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
)

// scaleHorizontally adds or removes nodes in a set of node sets to provide the required capacity in a tier.
func (ctx *Context) scaleHorizontally(
	nodeCapacity resources.NodeResources, // resources for each node in the tier/policy, as computed by the vertical autoscaler.
) resources.NodeSetsResources {
	totalRequiredCapacity := ctx.AutoscalingPolicyResult.RequiredCapacity.Total // total required resources, at the tier level.

	// The vertical autoscaler computed the expected capacity for each node in the autoscaling policy. The minimum number of nodes, specified by the user
	// in AutoscalingSpec.NodeCountRange.Min, can then be used to know what amount of resources we already have (AutoscalingSpec.NodeCountRange.Min * nodeCapacity).
	// nodesToAdd is the number of nodes to be added to that min. amount of resources to match the required capacity.
	var nodesToAdd int32

	// Scale horizontally to match memory requirements
	if !totalRequiredCapacity.Memory.IsZero() {
		nodeMemory := nodeCapacity.GetRequest(corev1.ResourceMemory)
		nodesToAdd = ctx.getNodesToAdd(nodeMemory.Value(), totalRequiredCapacity.Memory.Value(), string(corev1.ResourceMemory))
	}

	// Scale horizontally to match storage requirements
	if !totalRequiredCapacity.Storage.IsZero() {
		nodesToAdd = max(nodesToAdd, ctx.getNodesToAddForStorage(nodeCapacity, totalRequiredCapacity.Storage))
	}

	totalNodes := nodesToAdd + ctx.AutoscalingSpec.NodeCountRange.Min
	ctx.Log.Info("Horizontal autoscaler", "policy", ctx.AutoscalingSpec.Name,
		"scope", "tier",
		"count", totalNodes,
		"required_capacity", totalRequiredCapacity,
	)

	nodeSetsResources := resources.NewNodeSetsResources(ctx.AutoscalingSpec.Name, ctx.NodeSets.Names())
	nodeSetsResources.NodeResources = nodeCapacity
	distributeFairly(nodeSetsResources.NodeSetNodeCount, totalNodes)

	return nodeSetsResources
}

// getNodesToAddForStorage is wrapper around getNodesToAdd to handle some specificities of the storage resource.
// Because the Elasticsearch storage deciders require at least the total observed storage capacity we need to handle
// the following situations:
// * the volume capacity of the provisioned volume is greater than the one claimed.
// * the volume capacity of the provisioned volume is greater than the max storage capacity specified by the user.
// This function is only applicable to storage as it would otherwise prevent scale down of other resources.
func (ctx *Context) getNodesToAddForStorage(
	nodeCapacity resources.NodeResources,
	requiredTotalStorageCapacity *client.AutoscalingCapacity,
) int32 {
	totalCurrentCapacity := ctx.AutoscalingPolicyResult.CurrentCapacity.Total // total capacity as observed by Elasticsearch
	currentNodes := len(ctx.AutoscalingPolicyResult.CurrentNodes)
	if totalCurrentCapacity.Storage.Value() > int64(currentNodes)*ctx.AutoscalingSpec.StorageRange.Max.Value() {
		// The current storage capacity exceeds the maximum expected one. Since other other resources maybe scaled linearly
		// according to the storage capacity it may lead to an ineffective scaling of other resources.
		// See https://github.com/elastic/cloud-on-k8s/issues/4469
		ctx.Log.Info(
			"Current total storage capacity is greater than the one specified in the autoscaling specification.",
			"policy", ctx.AutoscalingSpec.Name,
			"scope", "tier",
			"resource", "storage",
			"current_total_storage_capacity", totalCurrentCapacity.Storage.Value(),
			"max_storage_capacity_per_node", ctx.AutoscalingSpec.StorageRange.Max.Value(),
			"current_node_count", currentNodes,
		)

		// Also surface this situation in the status.
		ctx.StatusBuilder.
			ForPolicy(ctx.AutoscalingSpec.Name).
			RecordEvent(
				status.UnexpectedStorageCapacity,
				fmt.Sprintf(
					"Current total storage capacity is %d, it is greater than the maximum expected one: %d (%d nodes * %d)",
					totalCurrentCapacity.Storage.Value(),
					int64(currentNodes)*ctx.AutoscalingSpec.StorageRange.Max.Value(),
					currentNodes,
					ctx.AutoscalingSpec.StorageRange.Max.Value(),
				),
			)
	}

	currentResources, hasStatus := ctx.CurrentAutoscalingStatus.CurrentResourcesForPolicy(ctx.AutoscalingSpec.Name)
	if !hasStatus ||
		requiredTotalStorageCapacity.Value() > totalCurrentCapacity.Storage.Value() {
		// We are in one of the following situation:
		// * The status is empty, this might happen if the autoscaling controller never ran on this cluster.
		// * The total required capacity (at the policy level) is greater than the observed capacity, we should scale up.
		nodeStorage := nodeCapacity.GetRequest(corev1.ResourceStorage)
		return ctx.getNodesToAdd(
			nodeStorage.Value(),
			requiredTotalStorageCapacity.Value(),
			string(corev1.ResourceStorage),
		)
	}

	// reuse the actual number of nodes from the status
	totalExpected := currentResources.NodeSetNodeCount.TotalNodeCount()
	// we still want to downscale if user required it.
	if totalExpected > ctx.AutoscalingSpec.NodeCountRange.Max {
		totalExpected = ctx.AutoscalingSpec.NodeCountRange.Max
	}
	if totalExpected > ctx.AutoscalingSpec.NodeCountRange.Min {
		return totalExpected - ctx.AutoscalingSpec.NodeCountRange.Min
	}

	return 0
}

// getNodesToAdd calculates the number of nodes to add in order to comply with the capacity requested by Elasticsearch.
func (ctx *Context) getNodesToAdd(
	nodeResourceCapacity int64, // resource capacity of a single node, for example the memory of a node in the tier
	totalRequiredCapacity int64, // required capacity at the tier level
	resourceName string, // used for logging and in events
) int32 {
	minNodes := ctx.AutoscalingSpec.NodeCountRange.Min
	// minResourceQuantity is the resource quantity in the tier before scaling horizontally.
	minResourceQuantity := int64(minNodes) * nodeResourceCapacity
	// resourceDelta holds the resource needed to comply with what is requested by Elasticsearch.
	resourceDelta := totalRequiredCapacity - minResourceQuantity
	// getNodeDelta translates resourceDelta into a number of nodes.
	nodeToAdd := getNodeDelta(resourceDelta, nodeResourceCapacity)

	maxNodes := ctx.AutoscalingSpec.NodeCountRange.Max
	if minNodes+nodeToAdd > maxNodes {
		// We would need to exceed the node count limit to fulfil the resource requirement.
		ctx.Log.Info(
			fmt.Sprintf("Can't provide total required %s", resourceName),
			"policy", ctx.AutoscalingSpec.Name,
			"scope", "tier",
			"resource", resourceName,
			"node_value", nodeResourceCapacity,
			"requested_value", totalRequiredCapacity,
			"requested_count", minNodes+nodeToAdd,
			"max_count", maxNodes,
		)

		// Also surface this situation in the status.
		ctx.StatusBuilder.
			ForPolicy(ctx.AutoscalingSpec.Name).
			RecordEvent(
				status.HorizontalScalingLimitReached,
				fmt.Sprintf("Can't provide total required %s %d, max number of nodes is %d, requires %d nodes", resourceName, totalRequiredCapacity, maxNodes, minNodes+nodeToAdd),
			)
		// Adjust the number of nodes to be added to comply with the limit specified by the user.
		nodeToAdd = maxNodes - minNodes
	}
	return nodeToAdd
}

// getNodeDelta computes the nodes to be added given a delta (the additional amount of resource needed)
// and the individual capacity a single node.
func getNodeDelta(delta, nodeCapacity int64) int32 {
	var nodeToAdd int32
	if delta < 0 {
		return 0
	}

	for delta > 0 {
		delta -= nodeCapacity
		// Compute how many nodes should be added
		nodeToAdd++
	}
	return nodeToAdd
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
