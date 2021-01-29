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
	"k8s.io/apimachinery/pkg/api/resource"
)

var giga = int64(1024 * 1024 * 1024)

// nodeResources computes the desired amount of memory and storage for a node managed by a given AutoscalingPolicySpec.
func (ctx *Context) nodeResources(minNodesCount int64, currentStorage resource.Quantity) resources.NodeResources {
	nodeResources := resources.NodeResources{}

	// Compute desired memory quantity for the nodes managed by this AutoscalingPolicySpec.
	if !ctx.RequiredCapacity.Node.Memory.IsEmpty() {
		memoryRequest := ctx.getResourceValue(
			ctx.AutoscalingSpec.Name,
			"memory",
			ctx.RequiredCapacity.Node.Memory,
			ctx.RequiredCapacity.Total.Memory,
			minNodesCount,
			ctx.AutoscalingSpec.Memory.Min,
			ctx.AutoscalingSpec.Memory.Max,
		)
		nodeResources.SetRequest(corev1.ResourceMemory, memoryRequest)
	}

	// Compute desired storage quantity for the nodes managed by this AutoscalingPolicySpec.
	if !ctx.RequiredCapacity.Node.Storage.IsEmpty() {
		storageRequest := ctx.getResourceValue(
			ctx.AutoscalingSpec.Name,
			"storage",
			ctx.RequiredCapacity.Node.Storage,
			ctx.RequiredCapacity.Total.Storage,
			minNodesCount,
			ctx.AutoscalingSpec.Storage.Min,
			ctx.AutoscalingSpec.Storage.Max,
		)
		if storageRequest.Cmp(currentStorage) < 0 {
			// Do not decrease storage capacity
			storageRequest = currentStorage
		}
		nodeResources.SetRequest(corev1.ResourceStorage, storageRequest)
	}

	// If no memory has been returned by the autoscaling API, but the user has expressed the intent to manage memory
	// using the autoscaling specification then we derive the memory from the storage if available.
	// See https://github.com/elastic/cloud-on-k8s/issues/4076
	if !nodeResources.HasRequest(corev1.ResourceMemory) && ctx.AutoscalingSpec.IsMemoryDefined() &&
		ctx.AutoscalingSpec.IsStorageDefined() && nodeResources.HasRequest(corev1.ResourceStorage) {
		nodeResources.SetRequest(corev1.ResourceMemory, memoryFromStorage(nodeResources.GetRequest(corev1.ResourceStorage), *ctx.AutoscalingSpec.Storage, *ctx.AutoscalingSpec.Memory))
	}

	// Same as above, if CPU limits have been expressed by the user in the autoscaling specification then we adjust CPU request according to the memory request.
	// See https://github.com/elastic/cloud-on-k8s/issues/4021
	if ctx.AutoscalingSpec.IsCPUDefined() && ctx.AutoscalingSpec.IsMemoryDefined() && nodeResources.HasRequest(corev1.ResourceMemory) {
		nodeResources.SetRequest(corev1.ResourceCPU, cpuFromMemory(nodeResources.GetRequest(corev1.ResourceMemory), *ctx.AutoscalingSpec.Memory, *ctx.AutoscalingSpec.CPU))
	}

	return nodeResources
}

// getResourceValue calculates the desired quantity for a specific resource for a node in a tier. This value is
// calculated according to the required value from the Elasticsearch autoscaling API and the resource constraints (limits)
// set by the user in the autoscaling specification.
func (ctx *Context) getResourceValue(
	autoscalingPolicyName, resourceType string,
	nodeRequired *client.AutoscalingCapacity, // node required capacity as returned by the Elasticsearch API
	totalRequired *client.AutoscalingCapacity, // tier required capacity as returned by the Elasticsearch API, considered as optional
	minNodesCount int64, // the minimum of nodes that will be deployed
	min, max resource.Quantity, // as expressed by the user
) resource.Quantity {
	if nodeRequired.IsZero() && totalRequired.IsZero() {
		// Elasticsearch has returned 0 for both the node and the tier level. Scale down resources to minimum.
		return resourceToQuantity(min.Value())
	}

	// Surface the situation where a resource is exhausted.
	if nodeRequired.Value() > max.Value() {
		// Elasticsearch requested more capacity per node than allowed by the user
		err := fmt.Errorf("node required %s is greater than the maximum one", resourceType)
		ctx.Log.Error(
			err, err.Error(),
			"scope", "node",
			"policy", autoscalingPolicyName,
			"required_"+resourceType, nodeRequired,
			"max_allowed_memory", max.Value(),
		)
		// Also update the autoscaling status accordingly
		ctx.StatusBuilder.
			ForPolicy(autoscalingPolicyName).
			WithEvent(
				status.VerticalScalingLimitReached,
				fmt.Sprintf("Node required %s %d is greater than max allowed: %d", resourceType, nodeRequired, max.Value()),
			)
	}

	nodeResource := nodeRequired.Value()
	if minNodesCount == 0 {
		// Elasticsearch returned some resources, even if user allowed empty nodeSet we need at least 1 node to host them.
		minNodesCount = 1
	}
	// Adjust the node requested capacity to try to fit the tier requested capacity.
	// This is done to check if the required resources at the tier level can fit on the minimum number of nodes scaled to
	// their maximums, and thus avoid to scale horizontally while scaling vertically to the maximum is enough.
	if totalRequired != nil && minNodesCount > 0 {
		memoryOverAllTiers := (*totalRequired).Value() / minNodesCount
		nodeResource = max64(nodeResource, memoryOverAllTiers)
	}

	// Try to round up the Gb value
	nodeResource = roundUp(nodeResource, giga)

	// Always ensure that the calculated resource quantity is at least equal to the min. limit provided by the user.
	if nodeResource < min.Value() {
		nodeResource = min.Value()
	}

	// Resource has been rounded up or scaled up to meet the tier requirements. We need to check that those operations
	// do not result in a resource quantity which is greater than the max. limit set by the user.
	if nodeResource > max.Value() {
		nodeResource = max.Value()
	}

	return resourceToQuantity(nodeResource)
}

// resourceToQuantity attempts to convert a raw integer value into a human readable quantity.
func resourceToQuantity(nodeResource int64) resource.Quantity {
	var nodeQuantity resource.Quantity
	if nodeResource >= giga && nodeResource%giga == 0 {
		// When it's possible we may want to express the memory with a "human readable unit" like the the Gi unit
		nodeQuantity = resource.MustParse(fmt.Sprintf("%dGi", nodeResource/giga))
	} else {
		nodeQuantity = resource.NewQuantity(nodeResource, resource.DecimalSI).DeepCopy()
	}
	return nodeQuantity
}

func max64(x int64, others ...int64) int64 {
	max := x
	for _, other := range others {
		if other > max {
			max = other
		}
	}
	return max
}

// roundUp rounds a value up to an other one.
func roundUp(v, n int64) int64 {
	r := v % n
	if r == 0 {
		return v
	}
	return v + n - r
}
