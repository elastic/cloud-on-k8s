// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"reflect"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

func TestGetMLNodesSettings(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs v1alpha1.AutoscalingPolicySpecs
	}
	tests := []struct {
		name          string
		fields        fields
		wantNodes     int32
		wantMaxMemory string
	}{
		{
			name: "happy path",
			fields: fields{
				AutoscalingPolicySpecs: v1alpha1.AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "data-policy", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"data"}}},
						AutoscalingResources: v1alpha1.AutoscalingResources{
							MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("16Gi")},
							NodeCountRange: v1alpha1.CountRange{Min: 3, Max: 5},
						},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "ml-policy", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"ml"}}},
						AutoscalingResources: v1alpha1.AutoscalingResources{
							MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("8Gi")},
							NodeCountRange: v1alpha1.CountRange{Min: 0, Max: 7},
						},
					},
				},
			},
			wantMaxMemory: "8589934592b",
			wantNodes:     7,
		},
		{
			name: "no dedicated ml tier", // not supported at the time this test is written, but we still want to ensure we return correct values
			fields: fields{
				AutoscalingPolicySpecs: v1alpha1.AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "data-policy", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"data"}}},
						AutoscalingResources: v1alpha1.AutoscalingResources{
							MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("16Gi")},
							NodeCountRange: v1alpha1.CountRange{Min: 3, Max: 5},
						},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "ml-data-policy", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"data", "ml"}}},
						AutoscalingResources: v1alpha1.AutoscalingResources{
							MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("8Gi")},
							NodeCountRange: v1alpha1.CountRange{Min: 0, Max: 7},
						},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "ml-policy2", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"ml"}}},
						AutoscalingResources: v1alpha1.AutoscalingResources{
							MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("16Gi")},
							NodeCountRange: v1alpha1.CountRange{Min: 0, Max: 4},
						},
					},
				},
			},
			wantMaxMemory: "17179869184b",
			wantNodes:     11,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodes, gotMaxMemory := GetMLNodesSettings(tt.fields.AutoscalingPolicySpecs)
			if gotNodes != tt.wantNodes {
				t.Errorf("AutoscalingAnnotation.GetMLNodesSettings() gotNodes = %v, want %v", gotNodes, tt.wantNodes)
			}
			if gotMaxMemory != tt.wantMaxMemory {
				t.Errorf("AutoscalingAnnotation.GetMLNodesSettings() gotMaxMemory = %v, want %v", gotMaxMemory, tt.wantMaxMemory)
			}
		})
	}
}

func TestElasticsearch_GetAutoscaledNodeSets(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs v1alpha1.AutoscalingPolicySpecs
		Elasticsearch          Elasticsearch
	}
	tests := []struct {
		name   string
		fields fields

		// policy name -> node sets
		want map[string][]string
		err  *NodeSetConfigError
	}{
		{
			name: "Overlapping roles",
			fields: fields{
				AutoscalingPolicySpecs: []v1alpha1.AutoscalingPolicySpec{
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "data_hot_content", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"data_hot", "data_content"}}},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "data_warm_content", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"data_content", "data_warm"}}},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "ml", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"ml"}}},
					},
				},
				Elasticsearch: Elasticsearch{
					Spec: ElasticsearchSpec{
						Version: "7.11.0",
						NodeSets: []NodeSet{
							{
								Name: "nodeset-hot-content-1",
								Config: &commonv1.Config{Data: map[string]interface{}{
									"node.roles": []string{"data_hot", "data_content"},
								}},
							},
							{
								Name: "nodeset-warm-content",
								Config: &commonv1.Config{Data: map[string]interface{}{
									"node.roles": []string{"data_warm", "data_content"},
								}},
							},
							{
								Name: "nodeset-hot-content-2",
								Config: &commonv1.Config{Data: map[string]interface{}{
									"node.roles": []string{"data_hot", "data_content"},
								}},
							},
							{
								Name: "nodeset-ml",
								Config: &commonv1.Config{Data: map[string]interface{}{
									"node.roles": []string{"ml"},
								}},
							},
							{
								Name: "masters",
								Config: &commonv1.Config{Data: map[string]interface{}{
									"node.roles": []string{"master"},
								}},
							},
						},
					},
				},
			},
			want: map[string][]string{
				"data_hot_content":  {"nodeset-hot-content-1", "nodeset-hot-content-2"},
				"data_warm_content": {"nodeset-warm-content"},
				"ml":                {"nodeset-ml"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.Elasticsearch.GetAutoscaledNodeSets(semver.MustParse(tt.fields.Elasticsearch.Spec.Version), tt.fields.AutoscalingPolicySpecs)
			assert.Equal(t, len(tt.want), len(got))
			for wantPolicy, wantNodeSets := range tt.want {
				gotNodeSets, hasNodeSets := got[wantPolicy]
				assert.True(t, hasNodeSets, "node set list was expected")
				assert.ElementsMatch(t, wantNodeSets, gotNodeSets.Names())
			}
			wantErr := tt.err != nil
			if (err != nil) != wantErr || (wantErr && !reflect.DeepEqual(err.Error(), tt.err.Error())) {
				t.Errorf("AutoscalingAnnotation.GetAutoscaledNodeSets() err = %v, want %v", err, tt.err)
			}
		})
	}
}
