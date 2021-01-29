// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var logTest = logf.Log.WithName("autoscaling-test")

func TestGetOfflineNodeSetsResources(t *testing.T) {
	type args struct {
		nodeSets                []string
		autoscalingSpec         esv1.AutoscalingPolicySpec
		actualAutoscalingStatus status.Status
	}
	tests := []struct {
		name string
		args args
		want resources.NodeSetsResources
	}{
		{
			name: "Do not scale down storage",
			args: args{
				nodeSets:        []string{"region-a", "region-b"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("2Gi", "6Gi").WithStorage("10Gi", "20Gi").Build(),
				actualAutoscalingStatus: status.Status{AutoscalingPolicyStatuses: []status.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []resources.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: resources.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}}}}},
			},
			want: resources.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []resources.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
				NodeResources:    resources.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}},
			},
		},
		{
			name: "Min. value has been increased by user",
			args: args{
				nodeSets:        []string{"region-a", "region-b"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("50Gi", "60Gi").WithStorage("10Gi", "20Gi").Build(),
				actualAutoscalingStatus: status.Status{AutoscalingPolicyStatuses: []status.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []resources.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: resources.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}}}}},
			},
			want: resources.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []resources.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
				NodeResources:    resources.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("50Gi" /* memory should be increased */), corev1.ResourceStorage: q("35Gi")}},
			},
		},
		{
			name: "New nodeSet is added by user while offline",
			args: args{
				nodeSets:        []string{"region-a", "region-b", "region-new"},
				autoscalingSpec: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(1, 6).WithMemory("2Gi", "6Gi").WithStorage("10Gi", "20Gi").Build(),
				actualAutoscalingStatus: status.Status{AutoscalingPolicyStatuses: []status.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []resources.NodeSetNodeCount{{Name: "region-a", NodeCount: 3}, {Name: "region-b", NodeCount: 3}},
					ResourcesSpecification: resources.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}}}}},
			},
			want: resources.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []resources.NodeSetNodeCount{{Name: "region-a", NodeCount: 2}, {Name: "region-b", NodeCount: 2}, {Name: "region-new", NodeCount: 2}},
				NodeResources:    resources.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("35Gi")}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetOfflineNodeSetsResources(logTest, tt.args.nodeSets, tt.args.autoscalingSpec, tt.args.actualAutoscalingStatus); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetOfflineNodeSetsResources() = %v, want %v", got, tt.want)
			}
		})
	}
}
