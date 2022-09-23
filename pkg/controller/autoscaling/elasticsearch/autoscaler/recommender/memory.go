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

type memory struct {
	base
	requiredNodeMemoryCapacity, requiredTotalMemoryCapacity *client.AutoscalingCapacity
}

func (m *memory) HasResourceRecommendation() bool {
	return true
}

func (m *memory) ManagedResource() corev1.ResourceName {
	return corev1.ResourceMemory
}

func (m *memory) NodeResourceQuantity() resource.Quantity {
	return getResourceValue(
		m.log,
		m.autoscalingSpec,
		m.statusBuilder,
		string(m.ManagedResource()),
		m.requiredNodeMemoryCapacity,
		m.requiredTotalMemoryCapacity,
		*m.autoscalingSpec.MemoryRange,
	)
}

func (m *memory) NodeCount(nodeCapacity v1alpha1.NodeResources) int32 {
	nodeMemory := nodeCapacity.GetRequest(corev1.ResourceMemory)
	return getNodeCount(
		m.log,
		m.autoscalingSpec,
		m.statusBuilder,
		string(m.ManagedResource()),
		nodeMemory.Value(),
		m.requiredTotalMemoryCapacity.Value(),
	)
}

func NewMemoryRecommender(
	log logr.Logger,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	autoscalingPolicyResult client.AutoscalingPolicyResult,
	currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus,
) (Recommender, error) {
	// Check if user expects the resource to be managed by the autoscaling controller
	hasResourceRange := autoscalingSpec.MemoryRange != nil

	// Did we get a resource requirement from Elasticsearch ?
	hasRequirement := !autoscalingPolicyResult.RequiredCapacity.Node.Memory.IsEmpty() ||
		!autoscalingPolicyResult.RequiredCapacity.Total.Memory.IsEmpty()

	if hasRequirement && autoscalingSpec.MemoryRange == nil {
		statusBuilder.ForPolicy(autoscalingSpec.Name).RecordEvent(v1alpha1.MemoryRequired, "Min and max memory must be specified")
		return nil, fmt.Errorf("min and max memory must be specified")
	}

	// We must recommend something in one of the following situations:
	// * User has provided a resource range for the resource.
	// * Elasticsearch API returned a non zero requirement
	if !hasResourceRange || !hasRequirement {
		return &nilRecommender{}, nil
	}

	memoryRecommender := memory{
		base: base{
			log:                      log,
			autoscalingSpec:          autoscalingSpec,
			statusBuilder:            statusBuilder,
			currentAutoscalingStatus: currentAutoscalingStatus,
		},
		requiredTotalMemoryCapacity: autoscalingPolicyResult.RequiredCapacity.Total.Memory,
		requiredNodeMemoryCapacity:  autoscalingPolicyResult.RequiredCapacity.Node.Memory,
	}

	return &memoryRecommender, nil
}
