// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

var logTest = logf.Log.WithName("autoscaling-test")

func TestGetOfflineNodeSetsResources(t *testing.T) {
	type args struct {
		nodeSets                 []string
		autoscalingSpec          v1alpha1.AutoscalingPolicySpec
		currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus
	}
	tests := []struct {
		name string
		args args
		want v1alpha1.NodeSetsResources
	}{
		{
			name: "Do not scale down storage",
			args: args{
				nodeSets:        []string{"region-a", "region-b"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("2Gi", "6Gi").WithStorage("10Gi", "20Gi").Build(),
				currentAutoscalingStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}}}}},
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi")},
				},
			},
		},
		{
			name: "Max. value has been decreased by the user, scale down memory",
			args: args{
				nodeSets:        []string{"region-a", "region-b"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("2Gi", "8Gi").WithStorage("10Gi", "20Gi").Build(),
				currentAutoscalingStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("10Gi"), corev1.ResourceStorage: q("20Gi")}}}}},
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8Gi"), corev1.ResourceStorage: q("20Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8Gi")},
				},
			},
		},
		{
			name: "Min. value has been increased by user",
			args: args{
				nodeSets:        []string{"region-a", "region-b"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("50Gi", "60Gi").WithStorage("10Gi", "20Gi").Build(),
				currentAutoscalingStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}}}}},
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("50Gi" /* memory should be increased */), corev1.ResourceStorage: q("35Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("50Gi")},
				},
			},
		},
		{
			name: "New nodeSet is added by user while offline",
			args: args{
				nodeSets:        []string{"region-a", "region-b", "region-new"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("2Gi", "6Gi").WithStorage("10Gi", "20Gi").Build(),
				currentAutoscalingStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}}}}},
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "region-a", NodeCount: 2}, {Name: "region-b", NodeCount: 2}, {Name: "region-new", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi")},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetOfflineNodeSetsResources(logTest, tt.args.nodeSets, tt.args.autoscalingSpec, tt.args.currentAutoscalingStatus); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetOfflineNodeSetsResources() = %v, want %v", got, tt.want)
			}
		})
	}
}
