// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNodeResources_ToNodeSetResourcesWith(t *testing.T) {
	tests := []struct {
		name     string
		node     NodeResources
		existing commonv1.Resources
		want     commonv1.Resources
	}{
		{
			name: "overrides cpu and memory requests and limits when recommended",
			node: NodeResources{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3000m"),
					corev1.ResourceMemory: resource.MustParse("6Gi"),
				},
			},
			existing: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					CPU:    qPtr("500m"),
					Memory: qPtr("2Gi"),
				},
				Limits: commonv1.ResourceAllocations{
					CPU:    qPtr("1000m"),
					Memory: qPtr("2Gi"),
				},
			},
			want: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					CPU:    qPtr("2000m"),
					Memory: qPtr("4Gi"),
				},
				Limits: commonv1.ResourceAllocations{
					CPU:    qPtr("3000m"),
					Memory: qPtr("6Gi"),
				},
			},
		},
		{
			name: "preserves existing cpu values when only memory is recommended",
			node: NodeResources{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("5Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("7Gi"),
				},
			},
			existing: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					CPU:    qPtr("800m"),
					Memory: qPtr("2Gi"),
				},
				Limits: commonv1.ResourceAllocations{
					CPU:    qPtr("1200m"),
					Memory: qPtr("3Gi"),
				},
			},
			want: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					CPU:    qPtr("800m"),
					Memory: qPtr("5Gi"),
				},
				Limits: commonv1.ResourceAllocations{
					CPU:    qPtr("1200m"),
					Memory: qPtr("7Gi"),
				},
			},
		},
		{
			name: "keeps existing values when no cpu or memory recommendation exists",
			node: NodeResources{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("8Gi"),
				},
			},
			existing: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					CPU:    qPtr("1000m"),
					Memory: qPtr("2Gi"),
				},
				Limits: commonv1.ResourceAllocations{
					CPU:    qPtr("1500m"),
					Memory: qPtr("3Gi"),
				},
			},
			want: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					CPU:    qPtr("1000m"),
					Memory: qPtr("2Gi"),
				},
				Limits: commonv1.ResourceAllocations{
					CPU:    qPtr("1500m"),
					Memory: qPtr("3Gi"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.ToNodeSetResourcesWith(tt.existing)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNodeResources_UpdateLimits(t *testing.T) {
	type fields struct {
		Limits   corev1.ResourceList
		Requests corev1.ResourceList
	}
	type args struct {
		autoscalingResources AutoscalingResources
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   NodeResources
	}{
		{
			name: "CPU limit should be twice the request",
			args: args{
				autoscalingResources: AutoscalingResources{
					CPURange: &QuantityRange{
						RequestsToLimitsRatio: qPtr("2.0"),
					},
					MemoryRange: nil, // no ratio, use default which is 1 for memory
				},
			},
			fields: fields{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			},
			want: NodeResources{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("4"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			},
		},
		{
			name: "Memory limit should be twice the request",
			args: args{
				autoscalingResources: AutoscalingResources{
					MemoryRange: &QuantityRange{
						RequestsToLimitsRatio: qPtr("2.0"),
					},
					CPURange: nil, // no ratio, use default which is 1 for memory
				},
			},
			fields: fields{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			},
			want: NodeResources{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("16Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			},
		},
		{
			name: "No limit",
			args: args{
				autoscalingResources: AutoscalingResources{
					MemoryRange: &QuantityRange{
						RequestsToLimitsRatio: qPtr("0.0"),
					},
					CPURange: &QuantityRange{
						RequestsToLimitsRatio: qPtr("0.0"),
					},
				},
			},
			fields: fields{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			},
			want: NodeResources{
				Limits: map[corev1.ResourceName]resource.Quantity{},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nr := NodeResources{
				Limits:   tt.fields.Limits,
				Requests: tt.fields.Requests,
			}
			got := nr.UpdateLimits(tt.args.autoscalingResources)
			assert.True(
				t,
				apiequality.Semantic.DeepEqual(got.Requests, tt.want.Requests),
				"NodeResources.UpdateLimits(): unexpected requests, expected %s, got %s",
				tt.want.Requests,
				got.Requests,
			)
			assert.True(
				t,
				apiequality.Semantic.DeepEqual(got.Limits, tt.want.Limits),
				"NodeResources.UpdateLimits(): unexpected limits, expected %s, got %s",
				tt.want.Limits,
				got.Limits,
			)
		})
	}
}

func TestResourcesSpecification_MaxMerge(t *testing.T) {
	type fields struct {
		Limits   corev1.ResourceList
		Requests corev1.ResourceList
	}
	type args struct {
		other        corev1.ResourceRequirements
		resourceName corev1.ResourceName
		want         NodeResources
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "max is receiver",
			fields: fields{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourceCPU:    resource.MustParse("2000"),
				},
			},
			args: args{
				other: corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("4Gi"),
						corev1.ResourceCPU:    resource.MustParse("1000"),
					},
				},
				resourceName: corev1.ResourceMemory,
				want: NodeResources{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("8Gi"),
						corev1.ResourceCPU:    resource.MustParse("2000"),
					},
				},
			},
		},
		{
			name: "max is other",
			fields: fields{
				// receiver
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("4Gi"),
					corev1.ResourceCPU:    resource.MustParse("1000"),
				},
			},
			args: args{
				other: corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("2000"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("8Gi"),
						corev1.ResourceCPU:    resource.MustParse("2000"),
					},
				},
				resourceName: corev1.ResourceMemory,
				want: NodeResources{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("8Gi"),
						corev1.ResourceCPU:    resource.MustParse("1000"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := &NodeResources{
				Limits:   tt.fields.Limits,
				Requests: tt.fields.Requests,
			}
			rs.MaxMerge(tt.args.other, tt.args.resourceName)
			assert.True(t, apiequality.Semantic.DeepEqual(rs.Requests, tt.args.want.Requests), "Unexpected requests")
			assert.True(t, apiequality.Semantic.DeepEqual(rs.Limits, tt.args.want.Limits), "Unexpected limits")
		})
	}
}

func qPtr(quantity string) *resource.Quantity {
	q := resource.MustParse(quantity)
	return &q
}
