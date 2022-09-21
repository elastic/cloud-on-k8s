// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
)

func Test_GetResources(t *testing.T) {
	defaultNodeSets := esv1.NodeSetList{{
		Name: "default",
	}}
	type args struct {
		currentNodeSets  esv1.NodeSetList
		nodeSetsStatus   v1alpha1.ElasticsearchAutoscalerStatus
		requiredCapacity client.AutoscalingPolicyResult
		policy           v1alpha1.AutoscalingPolicySpec
	}
	tests := []struct {
		name            string
		args            args
		want            v1alpha1.NodeSetsResources
		wantPolicyState []v1alpha1.PolicyState
		wantErr         bool
	}{
		{
			name: "Warn user if observed storage capacity is unexpected", // see https://github.com/elastic/cloud-on-k8s/issues/4469
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("2Gi"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					// current capacities as observed by Elasticsearch
					currentNodeStorage("368Gi"). // despite the 1Gi claim, physical capacity as reported by Elasticsearch is ~368Gi
					currentTierStorage("736Gi"). // 2 nodes with storage capacity ~368Gi in the current tier
					// required storage capacity
					requiredNodeStorage("64Mi").  // biggest shard is 64Mi
					requiredTierStorage("736Gi"). // the 2 claims, of 1Gi each, have been bound to 2 volumes of ~368Gi
					observedNodes("default-0", "default-1").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(2, 5).WithMemory("2Gi", "8Gi").WithStorage("1Gi", "4Gi").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{
						/* storage is scaled vertically to its max capacity to match the total required storage capacity required from Elasticsearch at the policy level */
						corev1.ResourceStorage: q("4Gi"),
						/* same for memory */
						corev1.ResourceMemory: q("8Gi"),
					},
					Limits: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8Gi")},
				},
			},
			wantPolicyState: []v1alpha1.PolicyState{
				{
					// We expect a warning here because the user expects the operator to scale storage vertically, while the physical capacity diverges from the claimed one.
					Type:     v1alpha1.UnexpectedNodeStorageCapacity,
					Messages: []string{"Vertical Pod autoscaling is not supported: current node storage capacity 395136991232 is greater than the claimed capacity 1073741824"},
				},
			},
		},
		{
			name: "Do not warn user if observed storage capacity is unexpected but storage capacity is fixed",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("2Gi"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					// current capacities as observed by Elasticsearch
					currentNodeStorage("368Gi"). // despite the 1Gi claim, physical capacity as reported by Elasticsearch is ~368Gi
					currentTierStorage("736Gi"). // 2 nodes with storage capacity ~368Gi in the current tier
					// required storage capacity
					requiredNodeStorage("64Mi").  // biggest shard is 64Mi
					requiredTierStorage("736Gi"). // the 2 claims, of 1Gi each, have been bound to 2 volumes of ~368Gi
					observedNodes("default-0", "default-1").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(2, 5).WithMemory("2Gi", "2Gi").WithStorage("1Gi", "1Gi").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{
						/* storage is scaled vertically to its max capacity to match the total required storage capacity required from Elasticsearch at the policy level */
						corev1.ResourceStorage: q("1Gi"),
						/* same for memory */
						corev1.ResourceMemory: q("2Gi"),
					},
					Limits: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("2Gi")},
				},
			},
			wantPolicyState: nil, // No warning here because user does not expect the operator to scale vertically the resources.
		},
		{
			name: "Scale both vertically and horizontally to fulfil storage capacity request",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3G"), corev1.ResourceStorage: q("6Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					// current capacities as observed by Elasticsearch
					currentNodeStorage("960Mi").  // physical capacity as reported by Elasticsearch, a bit smaller than 1Gi
					currentTierStorage("1920Mi"). // 2 nodes with storage capacity ~1Gi in the current tier
					// required storage capacity as expressed by the deciders
					requiredNodeMemory("3Gi").requiredTierMemory("9Gi").
					requiredNodeStorage("7Gi").requiredTierStorage("43Gi").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemory("3Gi", "4Gi").WithStorage("5Gi", "10Gi").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 5}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi"), corev1.ResourceStorage: q("10Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3Gi")},
				},
			},
		},
		{
			name: "Scale storage vertically to handle total storage requirement",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("2Gi"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					currentNodeStorage("1020M"). // 1Gi (1073MB) * 0.95
					currentTierStorage("2040M"). // 1Gi (1073MB) * 0.95 * 2
					observedNodes("default-0", "default-1").
					requiredNodeStorage("600Mi"). // largest shard can still fit in the current 1Gi storage
					requiredTierStorage("2044M"). // Storage deciders want a bit more than the current observed storage
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(2, 3).WithMemory("2Gi", "4Gi").WithStorage("1Gi", "2Gi").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}}, // Only scale vertically, do not scale out.
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4Gi"), corev1.ResourceStorage: q("2Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4Gi")},
				},
			},
		},
		{
			name: "Scale storage vertically to handle large shard",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("2Gi"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					currentNodeStorage("1020M"). // 1Gi (1073MB) * 0.95
					currentTierStorage("2040M"). // 1Gi (1073MB) * 0.95 * 2
					observedNodes("default-0", "default-1").
					requiredNodeStorage("1022M"). // Scale up vertically to accommodate for a large shard.
					requiredTierStorage("2044M"). // Storage deciders want a bit more than the current observed storage.
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(2, 3).WithMemory("2Gi", "4Gi").WithStorage("1Gi", "2Gi").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 2}}, // Only scale vertically, do not scale out.
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4Gi"), corev1.ResourceStorage: q("2Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4Gi")},
				},
			},
		},
		{
			name: "Scale memory vertically",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("3G"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("6G").
					requiredTierMemory("15G").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemory("5G", "8G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("6Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("6Gi")},
				},
			},
		},
		{
			name: "Do not scale down storage capacity",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4G"), corev1.ResourceStorage: q("10G")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					// current capacities as observed by Elasticsearch
					currentNodeStorage("9000M").  // physical capacity of a single as reported by Elasticsearch, a bit smaller than 10G in the claim
					currentTierStorage("27000M"). // 3 nodes with an actual storage capacity of 9G in the current tier
					// required storage capacity as expressed by the deciders
					requiredNodeMemory("6G").requiredTierMemory("15G").
					requiredNodeStorage("1Gi").requiredTierStorage("3Gi").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemory("5G", "8G").WithStorage("1G", "20G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("6Gi"), corev1.ResourceStorage: q("10G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("6Gi")},
				},
			},
		},
		{
			name: "Scale existing nodes vertically up to the tier limit",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4G"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("6G").
					requiredTierMemory("21G").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemory("5G", "8G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7Gi")},
				},
			},
		},
		{
			name: "Scale both vertically and horizontally",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4G"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("6G").
					requiredTierMemory("48G").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemory("5G", "8G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 6}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8G")},
				},
			},
		},
		{
			name: "Do not exceed node count specified by the user",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4G"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("6G").
					requiredTierMemory("48G"). // would require 6 nodes, user set a node count limit to 5
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 5).WithMemory("5G", "8G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 5}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8G")},
				},
			},
			wantPolicyState: []v1alpha1.PolicyState{
				{
					Type:     "HorizontalScalingLimitReached",
					Messages: []string{"Can't provide total required memory 48000000000, max number of nodes is 5, requires 6 nodes"},
				},
			},
		},
		{
			name: "Do not exceed horizontal and vertical limits specified by the user",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("4G"), corev1.ResourceStorage: q("1Gi")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("8G").  // user set a limit to 5G / node
					requiredTierMemory("48G"). // would require 10
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemory("5G", "7G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 6}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G")},
				},
			},
			wantPolicyState: []v1alpha1.PolicyState{
				{
					Type:     "VerticalScalingLimitReached",
					Messages: []string{"memory required per node, 8000000000, is greater than the maximum allowed: 7000000000"},
				},
				{
					Type:     "HorizontalScalingLimitReached",
					Messages: []string{"Can't provide total required memory 48000000000, max number of nodes is 6, requires 7 nodes"},
				},
			},
		},
		{
			name: "Do not scale down if all nodes are not observed by Elasticsearch",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 6}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G"), corev1.ResourceStorage: q("6G")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					// current capacities as observed by Elasticsearch
					currentNodeStorage("5G").  // physical capacity of a single as reported by Elasticsearch, a bit smaller than 5G in the claim
					currentTierStorage("25G"). // 5 nodes observed by Elasticsearch in the tier
					// required capacity as expressed by the deciders
					requiredNodeStorage("1G").  // biggest shard is 1G
					requiredTierStorage("30G"). // only 5 nodes with 6G of storage each are seen
					observedNodes("default-0", "default-1", "default-2", "default-3", "default-4").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 9).WithMemory("5G", "7G").WithStorage("5G", "6G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 6}}, // do not scale down to 5 nodes
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G"), corev1.ResourceStorage: q("6G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G")},
				},
			},
			wantPolicyState: []v1alpha1.PolicyState{},
		},
		{
			name: "Scale down if requested by users even if all nodes are not observed by Elasticsearch",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "my-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 6}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G"), corev1.ResourceStorage: q("6G")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					currentNodeStorage("6G").requiredNodeStorage("1G").   // biggest shard is 1G
					requiredTierStorage("30G").currentTierStorage("30G"). // only 5 nodes with 6G of storage each are seen
					observedNodes("default-0", "default-1", "default-2", "default-3", "default-4").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 5).WithMemory("5G", "7G").WithStorage("5G", "6G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 5}}, // scale down to 5 nodes as requested by the user
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G"), corev1.ResourceStorage: q("6G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G")},
				},
			},
			wantPolicyState: []v1alpha1.PolicyState{},
		},
		{
			name: "Scale down ML nodes to 0",
			args: args{
				currentNodeSets: defaultNodeSets,
				nodeSetsStatus: v1alpha1.ElasticsearchAutoscalerStatus{AutoscalingPolicyStatuses: []v1alpha1.AutoscalingPolicyStatus{{
					Name:                   "ml-autoscaling-policy",
					NodeSetNodeCount:       []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 1}},
					ResourcesSpecification: v1alpha1.NodeResources{Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("8G"), corev1.ResourceStorage: q("1G")}}}},
				},
				requiredCapacity: newAutoscalingPolicyResultBuilder(). // ML decider sets all the resources to 0.
											currentNodeStorage("1G").requiredNodeStorage("0").
											currentTierStorage("1G").requiredTierStorage("0").
											observedNodes("default-0").
											build(),
				policy: NewAutoscalingSpecBuilder("ml-autoscaling-policy").WithNodeCounts(0, 3).WithMemory("7G", "9G").WithStorage("1G", "1G").Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "ml-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 0}}, // Scale down to 0 node as no resource is needed.
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G"), corev1.ResourceStorage: q("1G")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("7G")},
				},
			},
			wantPolicyState: []v1alpha1.PolicyState{},
		},
		{
			name: "Adjust limits",
			args: args{
				currentNodeSets: defaultNodeSets,
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("6G").requiredTierMemory("15G").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").
					WithNodeCounts(3, 6).
					WithMemoryAndRatio("5G", "8G", resource.MustParse("2.0")).
					Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("6Gi")},
					Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("12Gi")},
				},
			},
		},
		{
			name: "Remove memory limit",
			args: args{
				currentNodeSets: defaultNodeSets,
				requiredCapacity: newAutoscalingPolicyResultBuilder().
					requiredNodeMemory("6G").
					requiredTierMemory("15G").
					build(),
				policy: NewAutoscalingSpecBuilder("my-autoscaling-policy").WithNodeCounts(3, 6).WithMemoryAndRatio("5G", "8G", resource.MustParse("0.0")).Build(),
			},
			want: v1alpha1.NodeSetsResources{
				Name:             "my-autoscaling-policy",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "default", NodeCount: 3}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: q("6Gi")},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewContext(
				logTest,
				tt.args.policy,
				tt.args.currentNodeSets,
				tt.args.nodeSetsStatus,
				tt.args.requiredCapacity,
				v1alpha1.NewAutoscalingStatusBuilder(),
			)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("autoscaler.NewContext(...) error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if got := ctx.GetResources(); !equality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("autoscaler.GetResources() = %v, want %v", got, tt.want)
			}
			gotStatus := ctx.StatusBuilder.Build()
			assert.ElementsMatch(t, getPolicyStates(gotStatus, "my-autoscaling-policy"), tt.wantPolicyState)
		})
	}
}

func getPolicyStates(status v1alpha1.ElasticsearchAutoscalerStatus, policyName string) []v1alpha1.PolicyState {
	for _, state := range status.AutoscalingPolicyStatuses {
		if state.Name == policyName {
			return state.PolicyStates
		}
	}
	return nil
}

// - AutoscalingSpec builder

type AutoscalingSpecBuilder struct {
	name                       string
	nodeCountMin, nodeCountMax int32
	cpu, memory, storage       *v1alpha1.QuantityRange
}

func NewAutoscalingSpecBuilder(name string) *AutoscalingSpecBuilder {
	return &AutoscalingSpecBuilder{name: name}
}

func (asb *AutoscalingSpecBuilder) WithNodeCounts(min, max int) *AutoscalingSpecBuilder {
	asb.nodeCountMin = int32(min)
	asb.nodeCountMax = int32(max)
	return asb
}

func (asb *AutoscalingSpecBuilder) WithMemory(min, max string) *AutoscalingSpecBuilder {
	asb.memory = &v1alpha1.QuantityRange{
		Min: resource.MustParse(min),
		Max: resource.MustParse(max),
	}
	return asb
}

func (asb *AutoscalingSpecBuilder) WithMemoryAndRatio(min, max string, ratio resource.Quantity) *AutoscalingSpecBuilder {
	asb.memory = &v1alpha1.QuantityRange{
		Min:                   resource.MustParse(min),
		Max:                   resource.MustParse(max),
		RequestsToLimitsRatio: &ratio,
	}
	return asb
}

func (asb *AutoscalingSpecBuilder) WithStorage(min, max string) *AutoscalingSpecBuilder {
	asb.storage = &v1alpha1.QuantityRange{
		Min: resource.MustParse(min),
		Max: resource.MustParse(max),
	}
	return asb
}

func (asb *AutoscalingSpecBuilder) WithCPU(min, max string) *AutoscalingSpecBuilder {
	asb.cpu = &v1alpha1.QuantityRange{
		Min: resource.MustParse(min),
		Max: resource.MustParse(max),
	}
	return asb
}

func (asb *AutoscalingSpecBuilder) WithCPUAndRatio(min, max string, ratio resource.Quantity) *AutoscalingSpecBuilder {
	asb.cpu = &v1alpha1.QuantityRange{
		Min:                   resource.MustParse(min),
		Max:                   resource.MustParse(max),
		RequestsToLimitsRatio: &ratio,
	}
	return asb
}

func (asb *AutoscalingSpecBuilder) Build() v1alpha1.AutoscalingPolicySpec {
	return v1alpha1.AutoscalingPolicySpec{
		NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{
			Name: asb.name,
		},
		AutoscalingResources: v1alpha1.AutoscalingResources{
			CPURange:     asb.cpu,
			MemoryRange:  asb.memory,
			StorageRange: asb.storage,
			NodeCountRange: v1alpha1.CountRange{
				Min: asb.nodeCountMin,
				Max: asb.nodeCountMax,
			},
		},
	}
}

// - PolicyCapacityInfo builder

type autoscalingPolicyResultBuilder struct {
	client.AutoscalingPolicyResult
}

func newAutoscalingPolicyResultBuilder() *autoscalingPolicyResultBuilder {
	return &autoscalingPolicyResultBuilder{}
}

func (rcb *autoscalingPolicyResultBuilder) build() client.AutoscalingPolicyResult {
	return rcb.AutoscalingPolicyResult
}

func (rcb *autoscalingPolicyResultBuilder) requiredNodeMemory(m string) *autoscalingPolicyResultBuilder {
	rcb.RequiredCapacity.Node.Memory = ptr(value(m))
	return rcb
}

func (rcb *autoscalingPolicyResultBuilder) requiredTierMemory(m string) *autoscalingPolicyResultBuilder {
	rcb.RequiredCapacity.Total.Memory = ptr(value(m))
	return rcb
}

func (rcb *autoscalingPolicyResultBuilder) requiredNodeStorage(m string) *autoscalingPolicyResultBuilder {
	rcb.RequiredCapacity.Node.Storage = ptr(value(m))
	return rcb
}

func (rcb *autoscalingPolicyResultBuilder) requiredTierStorage(m string) *autoscalingPolicyResultBuilder {
	rcb.RequiredCapacity.Total.Storage = ptr(value(m))
	return rcb
}

func (rcb *autoscalingPolicyResultBuilder) currentNodeStorage(m string) *autoscalingPolicyResultBuilder {
	rcb.CurrentCapacity.Node.Storage = ptr(value(m))
	return rcb
}

func (rcb *autoscalingPolicyResultBuilder) currentTierStorage(m string) *autoscalingPolicyResultBuilder {
	rcb.CurrentCapacity.Total.Storage = ptr(value(m))
	return rcb
}

func (rcb *autoscalingPolicyResultBuilder) observedNodes(nodes ...string) *autoscalingPolicyResultBuilder {
	rcb.CurrentNodes = make([]client.AutoscalingNodeInfo, len(nodes))
	for i := range nodes {
		rcb.CurrentNodes[i] = client.AutoscalingNodeInfo{Name: nodes[i]}
	}
	return rcb
}

func ptr(q int64) *client.AutoscalingCapacity {
	qPtr := resource.NewQuantity(q, resource.DecimalSI)
	v := client.AutoscalingCapacity{
		Quantity: *qPtr,
	}
	return &v
}

func value(v string) int64 {
	q := resource.MustParse(v)
	return q.Value()
}
