// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"reflect"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAutoscalingSpec_GetAutoscaledNodeSets(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs AutoscalingPolicySpecs
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
				AutoscalingPolicySpecs: []AutoscalingPolicySpec{
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "data_hot_content", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"data_hot", "data_content"}}},
					},
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "data_warm_content", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"data_content", "data_warm"}}},
					},
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"ml"}}},
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
			as := AutoscalingSpec{
				AutoscalingPolicySpecs: tt.fields.AutoscalingPolicySpecs,
				Elasticsearch:          tt.fields.Elasticsearch,
			}
			got, err := as.GetAutoscaledNodeSets()
			assert.Equal(t, len(tt.want), len(got))
			for wantPolicy, wantNodeSets := range tt.want {
				gotNodeSets, hasNodeSets := got[wantPolicy]
				assert.True(t, hasNodeSets, "node set list was expected")
				assert.ElementsMatch(t, wantNodeSets, gotNodeSets.Names())
			}
			if !reflect.DeepEqual(err, tt.err) {
				t.Errorf("AutoscalingSpec.GetAutoscaledNodeSets() err = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestAutoscalingSpec_GetMLNodesSettings(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs AutoscalingPolicySpecs
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
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml-policy", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"ml"}}},
						AutoscalingResources: AutoscalingResources{
							Memory:    &QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("8Gi")},
							NodeCount: CountRange{Min: 0, Max: 7},
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
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml-policy", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"data,ml"}}},
						AutoscalingResources: AutoscalingResources{
							Memory:    &QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("8Gi")},
							NodeCount: CountRange{Min: 0, Max: 7},
						},
					},
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml-policy2", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"ml"}}},
						AutoscalingResources: AutoscalingResources{
							Memory:    &QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("16Gi")},
							NodeCount: CountRange{Min: 0, Max: 4},
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
			as := AutoscalingSpec{AutoscalingPolicySpecs: tt.fields.AutoscalingPolicySpecs}
			gotNodes, gotMaxMemory := as.GetMLNodesSettings()
			if gotNodes != tt.wantNodes {
				t.Errorf("AutoscalingSpec.GetMLNodesSettings() gotNodes = %v, want %v", gotNodes, tt.wantNodes)
			}
			if gotMaxMemory != tt.wantMaxMemory {
				t.Errorf("AutoscalingSpec.GetMLNodesSettings() gotMaxMemory = %v, want %v", gotMaxMemory, tt.wantMaxMemory)
			}
		})
	}
}

func TestAutoscalingSpec_findByRoles(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs AutoscalingPolicySpecs
	}
	type args struct {
		roles []string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *AutoscalingPolicySpec
	}{
		{
			name: "Managed by an autoscaling policy",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					AutoscalingPolicySpec{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{
							Name: "ml_only",
							AutoscalingPolicy: AutoscalingPolicy{
								Roles: []string{"ml"},
							},
						},
					}},
			},
			args: args{roles: []string{"ml"}},
			want: &AutoscalingPolicySpec{
				NamedAutoscalingPolicy: NamedAutoscalingPolicy{
					Name: "ml_only",
					AutoscalingPolicy: AutoscalingPolicy{
						Roles: []string{"ml"},
					},
				},
			},
		},
		{
			name: "Not managed by an autoscaling policy",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					AutoscalingPolicySpec{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{
							Name: "ml_only",
							AutoscalingPolicy: AutoscalingPolicy{
								Roles: []string{"ml"},
							},
						},
					}},
			},
			args: args{roles: []string{"master"}},
			want: nil,
		},
		{
			name: "Not managed by an autoscaling policy",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					AutoscalingPolicySpec{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{
							Name: "ml_only",
							AutoscalingPolicy: AutoscalingPolicy{
								Roles: []string{"ml"},
							},
						},
					}},
			},
			args: args{roles: []string{"ml", "data"}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := AutoscalingSpec{
				AutoscalingPolicySpecs: tt.fields.AutoscalingPolicySpecs,
			}
			got := as.findByRoles(tt.args.roles)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AutoscalingSpec.findByRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
