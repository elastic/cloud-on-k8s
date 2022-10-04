// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package recommender

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/math"
)

// Recommender implements the logic to calculate the resource quantity required for a given resource at both node
// and tier (autoscaling policy) level.
type Recommender interface {
	// ManagedResource returns the type of resource managed by this recommender.
	ManagedResource() corev1.ResourceName
	// HasResourceRecommendation returns true if the recommender must be consulted.
	HasResourceRecommendation() bool
	// NodeResourceQuantity returns the advised Pod resource quantity to be set for this resource.
	NodeResourceQuantity() resource.Quantity
	// NodeCount returns the advised number of Pods required to fulfill the resource required by Elasticsearch given
	// resources allocated to a single node.
	NodeCount(nodeCapacity v1alpha1.NodeResources) int32
}

// base is a struct shared by all the recommenders.
type base struct {
	log                      logr.Logger
	statusBuilder            *v1alpha1.AutoscalingStatusBuilder
	autoscalingSpec          v1alpha1.AutoscalingPolicySpec
	currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus
}

// nilRecommender is a recommender which never return a recommendation.
type nilRecommender struct{}

func (n *nilRecommender) HasResourceRecommendation() bool {
	return false
}

func (n *nilRecommender) ManagedResource() corev1.ResourceName {
	return ""
}

func (n *nilRecommender) NodeResourceQuantity() resource.Quantity {
	return resource.Quantity{}
}

func (n *nilRecommender) TotalQuantity() resource.Quantity {
	return resource.Quantity{}
}

func (n *nilRecommender) NodeCount(_ v1alpha1.NodeResources) int32 {
	return 0
}

// getResourceValue calculates the desired quantity for a specific resource for a node in a tier. This value is
// calculated according to the required value from the Elasticsearch autoscaling API and the resource constraints (limits)
// set by the user in the autoscaling specification.
func getResourceValue(
	log logr.Logger,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	resourceType string,
	nodeRequired *client.AutoscalingCapacity, // node required capacity as returned by the Elasticsearch API
	totalRequired *client.AutoscalingCapacity, // tier required capacity as returned by the Elasticsearch API, considered as optional
	quantityRange v1alpha1.QuantityRange, // as expressed by the user
) resource.Quantity {
	max := quantityRange.Max.Value()
	// Surface the situation where a resource is exhausted.
	if nodeRequired.Value() > max {
		// Elasticsearch requested more capacity per node than allowed by the user
		err := fmt.Errorf("%s required per node is greater than the maximum one", resourceType)
		log.Error(
			err, err.Error(),
			"scope", "node",
			"policy", autoscalingSpec.Name,
			"required_"+resourceType, nodeRequired,
			"max_allowed_"+resourceType, max,
		)
		// Also update the autoscaling status accordingly
		statusBuilder.
			ForPolicy(autoscalingSpec.Name).
			RecordEvent(
				v1alpha1.VerticalScalingLimitReached,
				fmt.Sprintf("%s required per node, %d, is greater than the maximum allowed: %d", resourceType, nodeRequired.Value(), max),
			)
	}

	nodeResource := nodeRequired.Value()
	minNodesCount := int64(autoscalingSpec.NodeCountRange.Min)
	if minNodesCount == 0 {
		// Elasticsearch returned some resources, even if user allowed empty nodeSet we need at least 1 node to host them.
		minNodesCount = 1
	}
	// Adjust the node requested capacity to try to fit the tier requested capacity.
	// This is done to check if the required resources at the tier level can fit on the minimum number of nodes scaled to
	// their maximums, and thus avoid to scale horizontally while scaling vertically to the maximum is enough.
	if totalRequired != nil && minNodesCount > 0 {
		resourceOverAllTiers := (totalRequired).Value() / minNodesCount
		nodeResource = max64(nodeResource, resourceOverAllTiers)
	}

	// Try to round up to the next GiB value
	nodeResource = math.RoundUp(nodeResource, v1alpha1.GiB)

	// Always ensure that the calculated resource quantity is at least equal to the min. limit provided by the user.
	if nodeResource < quantityRange.Min.Value() {
		nodeResource = quantityRange.Min.Value()
	}

	// Resource has been rounded up or scaled up to meet the tier requirements. We need to check that those operations
	// do not result in a resource quantity which is greater than the max. limit set by the user.
	if nodeResource > max {
		nodeResource = max
	}

	return v1alpha1.ResourceToQuantity(nodeResource)
}

// getNodeCount calculates the number of nodes to deploy in a tier to comply with the capacity requested by Elasticsearch.
func getNodeCount(
	log logr.Logger,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	resourceName string, // used for logging and in events
	nodeResourceCapacity int64, // resource capacity of a single node, for example the memory of a node in the tier
	totalRequiredCapacity int64, // required capacity at the tier level
) int32 {
	minNodes := autoscalingSpec.NodeCountRange.Min
	// minResourceQuantity is the resource quantity in the tier before scaling horizontally.
	minResourceQuantity := int64(minNodes) * nodeResourceCapacity
	// resourceDelta holds the resource needed to comply with what is requested by Elasticsearch.
	resourceDelta := totalRequiredCapacity - minResourceQuantity
	// getNodeDelta translates resourceDelta into a number of nodes.
	nodeToAdd := getNodeDelta(resourceDelta, nodeResourceCapacity)

	maxNodes := autoscalingSpec.NodeCountRange.Max
	if minNodes+nodeToAdd > maxNodes {
		// We would need to exceed the node count limit to fulfil the resource requirement.
		log.Info(
			fmt.Sprintf("Can't provide total required %s", resourceName),
			"policy", autoscalingSpec.Name,
			"scope", "tier",
			"resource", resourceName,
			"node_value", nodeResourceCapacity,
			"requested_value", totalRequiredCapacity,
			"requested_count", minNodes+nodeToAdd,
			"max_count", maxNodes,
		)

		// Also surface this situation in the status.
		statusBuilder.
			ForPolicy(autoscalingSpec.Name).
			RecordEvent(
				v1alpha1.HorizontalScalingLimitReached,
				fmt.Sprintf("Can't provide total required %s %d, max number of nodes is %d, requires %d nodes", resourceName, totalRequiredCapacity, maxNodes, minNodes+nodeToAdd),
			)
	}
	// Adjust the number of nodes to be added to comply with the limit specified by the user.
	return autoscalingSpec.NodeCountRange.Enforce(minNodes + nodeToAdd)
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

func maxResource(a, b resource.Quantity) resource.Quantity {
	if a.Cmp(b) < 0 {
		return b
	}
	return a
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
