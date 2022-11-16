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
)

type cpu struct {
	base
	requiredNodeCPUCapacity, requiredTotalCPUCapacity *client.AutoscalingCapacity
}

func (m *cpu) HasResourceRecommendation() bool {
	return true
}

func (m *cpu) ManagedResource() corev1.ResourceName {
	return corev1.ResourceCPU
}

func (m *cpu) NodeResourceQuantity() resource.Quantity {
	max := m.autoscalingSpec.CPURange.Max.MilliValue()
	// Surface the situation where CPU is exhausted.
	if m.requiredNodeCPUCapacity.MilliValue() > max {
		// Elasticsearch requested more CPU per node than allowed by the user
		err := fmt.Errorf("CPU required per node is greater than the maximum allowed")
		m.log.Error(
			err, err.Error(),
			"scope", "node",
			"policy", m.autoscalingSpec.Name,
			"required_cpu", m.requiredNodeCPUCapacity,
			"max_allowed_cpu", m.autoscalingSpec.CPURange.Max,
		)
		// Also update the autoscaling status accordingly
		m.statusBuilder.
			ForPolicy(m.autoscalingSpec.Name).
			RecordEvent(
				v1alpha1.VerticalScalingLimitReached,
				fmt.Sprintf("Required CPU per node %s is greater than the maximum allowed: %s", m.requiredNodeCPUCapacity, m.autoscalingSpec.CPURange.Max.String()),
			)
	}

	nodeResource := m.requiredNodeCPUCapacity.MilliValue()
	minNodesCount := int64(m.autoscalingSpec.NodeCountRange.Min)
	if minNodesCount == 0 {
		// Elasticsearch returned some resources, even if user allowed empty nodeSet we need at least 1 node to host them.
		minNodesCount = 1
	}
	// Adjust the node requested capacity to try to fit the tier requested capacity.
	// This is done to check if the required resources at the tier level can fit on the minimum number of nodes scaled to
	// their maximums, and thus avoid to scale horizontally while scaling vertically to the maximum is enough.
	if m.requiredTotalCPUCapacity != nil && minNodesCount > 0 {
		resourceOverAllTiers := (m.requiredTotalCPUCapacity).MilliValue() / minNodesCount
		nodeResource = max64(nodeResource, resourceOverAllTiers)
	}

	// Always ensure that the calculated resource quantity is at least equal to the min. limit provided by the user.
	if nodeResource < m.autoscalingSpec.CPURange.Min.MilliValue() {
		nodeResource = m.autoscalingSpec.CPURange.Min.MilliValue()
	}

	// Resource has been rounded up or scaled up to meet the tier requirements. We need to check that those operations
	// do not result in a resource quantity which is greater than the max. limit set by the user.
	if nodeResource > max {
		nodeResource = max
	}
	quantity := resource.NewMilliQuantity(nodeResource, resource.DecimalSI)
	return *quantity
}

func (m *cpu) NodeCount(nodeCapacity v1alpha1.NodeResources) int32 {
	nodeCPU := nodeCapacity.GetRequest(corev1.ResourceCPU)
	return getNodeCount(
		m.log,
		m.autoscalingSpec,
		m.statusBuilder,
		string(m.ManagedResource()),
		nodeCPU.MilliValue(),
		m.requiredTotalCPUCapacity.MilliValue(),
	)
}

func NewCPURecommender(
	log logr.Logger,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	autoscalingPolicyResult client.AutoscalingPolicyResult,
	currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus,
) (Recommender, error) {
	// Check if user expects the resource to be managed by the autoscaling controller
	hasResourceRange := autoscalingSpec.CPURange != nil

	// Did we get a resource requirement from Elasticsearch?
	hasRequirement := !autoscalingPolicyResult.RequiredCapacity.Node.Processors.IsEmpty() ||
		!autoscalingPolicyResult.RequiredCapacity.Total.Processors.IsEmpty()

	if hasRequirement && autoscalingSpec.CPURange == nil {
		statusBuilder.ForPolicy(autoscalingSpec.Name).RecordEvent(v1alpha1.CPURequired, "Min and max CPU must be specified")
		return nil, fmt.Errorf("min and max CPU must be specified")
	}

	// We must recommend something in one of the following situations:
	// * User has provided a resource range for the resource.
	// * Elasticsearch API returned a non zero requirement
	if !hasResourceRange || !hasRequirement {
		return &nilRecommender{}, nil
	}

	cpuRecommender := cpu{
		base: base{
			log:                      log,
			autoscalingSpec:          autoscalingSpec,
			statusBuilder:            statusBuilder,
			currentAutoscalingStatus: currentAutoscalingStatus,
		},
		requiredTotalCPUCapacity: autoscalingPolicyResult.RequiredCapacity.Total.Processors,
		requiredNodeCPUCapacity:  autoscalingPolicyResult.RequiredCapacity.Node.Processors,
	}

	return &cpuRecommender, nil
}
