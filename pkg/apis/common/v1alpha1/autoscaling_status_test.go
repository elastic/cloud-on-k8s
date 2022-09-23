// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAutoscalingStatusBuilder_Build(t *testing.T) {
	tests := []struct {
		name       string
		newBuilder func() *AutoscalingStatusBuilder
		want       ElasticsearchAutoscalerStatus
	}{
		{
			name: "Not online yet, no reason",
			newBuilder: func() *AutoscalingStatusBuilder {
				asb := NewAutoscalingStatusBuilder()
				asb.ForPolicy("policy0").
					SetNodeSetsResources(NodeSetsResources{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						NodeResources: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
					})
				return asb
			},
			want: ElasticsearchAutoscalerStatus{
				Conditions: Conditions{
					Condition{
						Type:    ElasticsearchAutoscalerActive,
						Status:  corev1.ConditionTrue,
						Message: "",
					},
					Condition{
						Type:    ElasticsearchAutoscalerHealthy,
						Status:  corev1.ConditionFalse,
						Message: "An error prevented resource calculation from the Elasticsearch autoscaling API.",
					},
					Condition{
						Type:   ElasticsearchAutoscalerLimited,
						Status: corev1.ConditionFalse,
					},
					Condition{
						Type:   ElasticsearchAutoscalerOnline,
						Status: corev1.ConditionUnknown,
					},
				},
				AutoscalingPolicyStatuses: []AutoscalingPolicyStatus{
					{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
						PolicyStates: []PolicyState{},
					},
				},
			},
		},
		{
			name: "Offline",
			newBuilder: func() *AutoscalingStatusBuilder {
				asb := NewAutoscalingStatusBuilder().SetOnline(false, "Reason for not being online")
				asb.ForPolicy("policy0").
					SetNodeSetsResources(NodeSetsResources{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						NodeResources: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
					})
				return asb
			},
			want: ElasticsearchAutoscalerStatus{
				Conditions: Conditions{
					Condition{
						Type:    ElasticsearchAutoscalerActive,
						Status:  corev1.ConditionTrue,
						Message: "",
					},
					Condition{
						Type:    ElasticsearchAutoscalerHealthy,
						Status:  corev1.ConditionFalse,
						Message: "An error prevented resource calculation from the Elasticsearch autoscaling API.",
					},
					Condition{
						Type:   ElasticsearchAutoscalerLimited,
						Status: corev1.ConditionFalse,
					},
					Condition{
						Type:    ElasticsearchAutoscalerOnline,
						Status:  corev1.ConditionFalse,
						Message: "Reason for not being online",
					},
				},
				AutoscalingPolicyStatuses: []AutoscalingPolicyStatus{
					{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
						PolicyStates: []PolicyState{},
					},
				},
			},
		},
		{
			name: "Scaling limit reached",
			newBuilder: func() *AutoscalingStatusBuilder {
				asb := NewAutoscalingStatusBuilder().SetOnline(true, "Elasticsearch is available")
				asb.ForPolicy("policy0").
					SetNodeSetsResources(NodeSetsResources{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						NodeResources: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
					})
				asb.ForPolicy("policy1").
					RecordEvent(
						VerticalScalingLimitReached,
						"memory required per node, 8Gi, is greater than the maximum allowed: 7Gi",
					).SetNodeSetsResources(NodeSetsResources{
					Name:             "policy1",
					NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset1-0", NodeCount: 3}, {Name: "nodeset1-1", NodeCount: 2}},
					NodeResources: NodeResources{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
					},
				},
				)
				asb.ForPolicy("policy2").
					RecordEvent(
						VerticalScalingLimitReached,
						"memory required per node, 16Gi, is greater than the maximum allowed: 9Gi",
					).SetNodeSetsResources(NodeSetsResources{
					Name:             "policy1",
					NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset2-0", NodeCount: 2}, {Name: "nodeset2-1", NodeCount: 2}},
					NodeResources: NodeResources{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
					},
				},
				)
				return asb
			},
			want: ElasticsearchAutoscalerStatus{
				Conditions: Conditions{
					Condition{
						Type:    ElasticsearchAutoscalerActive,
						Status:  corev1.ConditionTrue,
						Message: "",
					},
					Condition{
						Type:    ElasticsearchAutoscalerHealthy,
						Status:  corev1.ConditionTrue,
						Message: "",
					},
					Condition{
						Type:    ElasticsearchAutoscalerLimited,
						Status:  corev1.ConditionTrue,
						Message: "Limit reached for policies policy1,policy2",
					},
					Condition{
						Type:    ElasticsearchAutoscalerOnline,
						Status:  corev1.ConditionTrue,
						Message: "Elasticsearch is available",
					},
				},
				AutoscalingPolicyStatuses: []AutoscalingPolicyStatus{
					{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
						PolicyStates: []PolicyState{},
					},
					{
						Name:             "policy1",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset1-0", NodeCount: 3}, {Name: "nodeset1-1", NodeCount: 2}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
						},
						PolicyStates: []PolicyState{
							{
								Type:     VerticalScalingLimitReached,
								Messages: []string{"memory required per node, 8Gi, is greater than the maximum allowed: 7Gi"},
							},
						},
					},
					{
						Name:             "policy2",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset2-0", NodeCount: 2}, {Name: "nodeset2-1", NodeCount: 2}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
						},
						PolicyStates: []PolicyState{
							{
								Type:     VerticalScalingLimitReached,
								Messages: []string{"memory required per node, 16Gi, is greater than the maximum allowed: 9Gi"},
							},
						},
					},
				},
			},
		},
		{
			name: "Not online with policy errors",
			newBuilder: func() *AutoscalingStatusBuilder {
				asb := NewAutoscalingStatusBuilder().SetOnline(false, "Elasticsearch is not available")
				asb.ForPolicy("policy0").
					RecordEvent(
						MemoryRequired,
						"min and max memory must be specified",
					).SetNodeSetsResources(NodeSetsResources{
					Name:             "policy0",
					NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
					NodeResources: NodeResources{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
					},
				},
				)
				asb.ForPolicy("policy1").
					RecordEvent(
						NoNodeSet,
						"no nodeSets for tier policy1",
					).SetNodeSetsResources(NodeSetsResources{
					Name:             "policy1",
					NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset1-0", NodeCount: 3}, {Name: "nodeset1-1", NodeCount: 2}},
					NodeResources: NodeResources{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
					},
				},
				)
				asb.ForPolicy("policy2").
					RecordEvent(
						VerticalScalingLimitReached,
						"memory required per node, 16Gi, is greater than the maximum allowed: 9Gi",
					).SetNodeSetsResources(NodeSetsResources{
					Name:             "policy1",
					NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset2-0", NodeCount: 2}, {Name: "nodeset2-1", NodeCount: 2}},
					NodeResources: NodeResources{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
					},
				},
				)
				return asb
			},
			want: ElasticsearchAutoscalerStatus{
				Conditions: Conditions{
					Condition{
						Type:    ElasticsearchAutoscalerActive,
						Status:  corev1.ConditionTrue,
						Message: "",
					},
					Condition{
						Type:    ElasticsearchAutoscalerHealthy,
						Status:  corev1.ConditionFalse,
						Message: "An error prevented resource calculation from the Elasticsearch autoscaling API. Issues reported for the following policies: [policy0,policy1]. Check operator logs, Kubernetes events, and policies status for more details",
					},
					Condition{
						Type:    ElasticsearchAutoscalerLimited,
						Status:  corev1.ConditionTrue,
						Message: "Limit reached for policies policy2",
					},
					Condition{
						Type:    ElasticsearchAutoscalerOnline,
						Status:  corev1.ConditionFalse,
						Message: "Elasticsearch is not available",
					},
				},
				AutoscalingPolicyStatuses: []AutoscalingPolicyStatus{
					{
						Name:             "policy0",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset0-0", NodeCount: 1}, {Name: "nodeset0-1", NodeCount: 1}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2000m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("3")},
						},
						PolicyStates: []PolicyState{
							{
								Type:     MemoryRequired,
								Messages: []string{"min and max memory must be specified"},
							},
						},
					},
					{
						Name:             "policy1",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset1-0", NodeCount: 3}, {Name: "nodeset1-1", NodeCount: 2}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
						},
						PolicyStates: []PolicyState{
							{
								Type:     NoNodeSet,
								Messages: []string{"no nodeSets for tier policy1"},
							},
						},
					},
					{
						Name:             "policy2",
						NodeSetNodeCount: NodeSetNodeCountList{{Name: "nodeset2-0", NodeCount: 2}, {Name: "nodeset2-1", NodeCount: 2}},
						ResourcesSpecification: NodeResources{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("2500m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("5")},
						},
						PolicyStates: []PolicyState{
							{
								Type:     VerticalScalingLimitReached,
								Messages: []string{"memory required per node, 16Gi, is greater than the maximum allowed: 9Gi"},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asb := tt.newBuilder().Build()
			cmpConditions(t, tt.want.Conditions, asb.Conditions)
			cmpAutoscalingPolicyStatuses(t, tt.want.AutoscalingPolicyStatuses, asb.AutoscalingPolicyStatuses)
		})
	}
}

func cmpConditions(t *testing.T, expectedConditions, gotConditions Conditions) {
	t.Helper()
	for _, expected := range expectedConditions {
		idx := gotConditions.Index(expected.Type)
		if idx < 0 {
			t.Errorf("Expected condition %s not found", expected.Type)
			continue
		}
		gotCondition := gotConditions[idx]
		assert.Equal(t, expected.Status, gotCondition.Status, "for condition type %s", gotCondition.Type)
		assert.Equal(t, expected.Message, gotCondition.Message, "for condition type  %s", gotCondition.Type)
	}
}

func cmpAutoscalingPolicyStatuses(t *testing.T, expectedPolicyStatuses, gotPolicyStatuses []AutoscalingPolicyStatus) {
	t.Helper()
	for _, expected := range expectedPolicyStatuses {
		gotPolicyStatus := getPolicyStatus(expected.Name, gotPolicyStatuses)
		if gotPolicyStatus == nil {
			t.Errorf("Expected policy status %s not found", expected.Name)
			continue
		}
		assert.Equal(t, expected.NodeSetNodeCount, gotPolicyStatus.NodeSetNodeCount, "for policy %s", expected.Name)
		assert.Equal(t, expected.PolicyStates, gotPolicyStatus.PolicyStates, "for policy %s", expected.Name)
		if !equality.Semantic.DeepEqual(expected.ResourcesSpecification, gotPolicyStatus.ResourcesSpecification) {
			t.Errorf("for policy %s: expected resources %v, got %v", expected.Name, expected.ResourcesSpecification, gotPolicyStatus.ResourcesSpecification)
		}
	}
}

func getPolicyStatus(policyName string, policies []AutoscalingPolicyStatus) *AutoscalingPolicyStatus {
	for i := range policies {
		if policies[i].Name == policyName {
			return &policies[i]
		}
	}
	return nil
}
