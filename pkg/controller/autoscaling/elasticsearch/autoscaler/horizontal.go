// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	corev1 "k8s.io/api/core/v1"
)

// scaleHorizontally adds or removes nodes in a set of node sets to provide the required capacity in a tier.
func (ctx *Context) scaleHorizontally(
	nodeCapacity resources.NodeResources, // resources for each node in the tier/policy, as computed by the vertical autoscaler.
) resources.NodeSetsResources {
	totalRequiredCapacity := ctx.AutoscalingPolicyResult.RequiredCapacity.Total // total required resources, at the tier level.

	// The vertical autoscaler computed the expected capacity for each node in the autoscaling policy. The minimum number of nodes, specified by the user
	// in AutoscalingSpec.NodeCount.Min, can then be used to know what amount of resources we already have (AutoscalingSpec.NodeCount.Min * nodeCapacity).
	// nodesToAdd is the number of nodes to be added to that min. amount of resources to match the required capacity.
	var nodesToAdd int32

	// Scale horizontally to match memory requirements
	if !totalRequiredCapacity.Memory.IsZero() {
		nodeMemory := nodeCapacity.GetRequest(corev1.ResourceMemory)
		nodesToAdd = ctx.getNodesToAdd(nodeMemory.Value(), totalRequiredCapacity.Memory.Value(), ctx.AutoscalingSpec.NodeCount.Min, ctx.AutoscalingSpec.NodeCount.Max, string(corev1.ResourceMemory))
	}

	// Scale horizontally to match storage requirements
	if !totalRequiredCapacity.Storage.IsZero() {
		nodeStorage := nodeCapacity.GetRequest(corev1.ResourceStorage)
		nodesToAdd = max(nodesToAdd, ctx.getNodesToAdd(nodeStorage.Value(), totalRequiredCapacity.Storage.Value(), ctx.AutoscalingSpec.NodeCount.Min, ctx.AutoscalingSpec.NodeCount.Max, string(corev1.ResourceStorage)))
	}

	totalNodes := nodesToAdd + ctx.AutoscalingSpec.NodeCount.Min
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

// getNodesToAdd calculates the number of nodes to add in order to comply with the capacity requested by Elasticsearch.
func (ctx *Context) getNodesToAdd(
	nodeResourceCapacity int64, // resource capacity of a single node, for example the memory of a node in the tier
	totalRequiredCapacity int64, // required capacity at the tier level
	minNodes, maxNodes int32, // min and max number of nodes in this tier, as specified by the user the autoscaling spec.
	resourceName string, // used for logging and in events
) int32 {
	// minResourceQuantity is the resource quantity in the tier before scaling horizontally.
	minResourceQuantity := int64(minNodes) * nodeResourceCapacity
	// resourceDelta holds the resource needed to comply with what is requested by Elasticsearch.
	resourceDelta := totalRequiredCapacity - minResourceQuantity
	// getNodeDelta translates resourceDelta into a number of nodes.
	nodeToAdd := getNodeDelta(resourceDelta, nodeResourceCapacity)

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
