// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package resources

import (
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeResources_ToContainerResourcesWith(t *testing.T) {
	type fields struct {
		Limits   corev1.ResourceList
		Requests corev1.ResourceList
	}
	type args struct {
		into corev1.ResourceRequirements
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   corev1.ResourceRequirements
	}{
		{
			name: "Source requirements are nil",
			fields: fields{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Limits: map[corev1.ResourceName]resource.Quantity{
					// corev1.ResourceCPU is not set and should not be present in the result.
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			args: args{into: corev1.ResourceRequirements{
				Requests: nil,
				Limits:   nil,
			}},
			want: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Limits: map[corev1.ResourceName]resource.Quantity{
					// corev1.ResourceCPU is not expected
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		{
			name: "Remove a requirements if not present",
			fields: fields{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Limits: map[corev1.ResourceName]resource.Quantity{
					// corev1.ResourceCPU is not expected
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			args: args{into: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("2"), // should be removed in the result
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			}},
			want: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Limits: map[corev1.ResourceName]resource.Quantity{
					// corev1.ResourceCPU is not expected
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		{
			name: "Do not delete extended resource",
			fields: fields{
				Limits:   nil,
				Requests: nil,
			},
			args: args{into: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("4"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("8"),
				},
			}},
			want: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("4"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("8"),
				},
			},
		},
		{
			name: "Merge with extended resource",
			fields: fields{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			args: args{into: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("4"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("8"),
				},
			}},
			want: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("4"),
					corev1.ResourceMemory:     resource.MustParse("8Gi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					"requests.nvidia.com/gpu": resource.MustParse("8"),
					corev1.ResourceMemory:     resource.MustParse("8Gi"),
					corev1.ResourceCPU:        resource.MustParse("2"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nr := &NodeResources{
				Limits:   tt.fields.Limits,
				Requests: tt.fields.Requests,
			}
			if got := nr.ToContainerResourcesWith(tt.args.into); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeResources.ToContainerResourcesWith() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeResources_UpdateLimits(t *testing.T) {
	type fields struct {
		Limits   corev1.ResourceList
		Requests corev1.ResourceList
	}
	type args struct {
		autoscalingResources v1.AutoscalingResources
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
				autoscalingResources: v1.AutoscalingResources{
					CPU: &v1.QuantityRange{
						RequestsToLimitsRatio: float64ptr(2.0),
					},
					Memory: nil, // no ratio, use default which is 1 for memory
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
				autoscalingResources: v1.AutoscalingResources{
					Memory: &v1.QuantityRange{
						RequestsToLimitsRatio: float64ptr(2.0),
					},
					CPU: nil, // no ratio, use default which is 1 for memory
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
				autoscalingResources: v1.AutoscalingResources{
					Memory: &v1.QuantityRange{
						RequestsToLimitsRatio: float64ptr(0.0),
					},
					CPU: nil,
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

func TestNodeSetsResources_Match(t *testing.T) {
	type fields struct {
		Name                   string
		NodeSetNodeCount       NodeSetNodeCountList
		ResourcesSpecification NodeResources
	}
	type args struct {
		nodeSet esv1.NodeSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Volume claim does not exist in nodeSet spec",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: resource.MustParse("2Gi"), corev1.ResourceMemory: resource.MustParse("4Gi"), corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: false,
		},
		{
			name: "Volume claim are not equals",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: resource.MustParse("2Gi"), corev1.ResourceMemory: resource.MustParse("4Gi"), corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withStorageRequest("1Gi").withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: false,
		},
		{
			name: "Node count is not the same",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 6}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: resource.MustParse("2Gi"), corev1.ResourceMemory: resource.MustParse("4Gi"), corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withStorageRequest("2Gi").withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: false,
		},
		{
			name: "Memory is not equal",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: resource.MustParse("2Gi"), corev1.ResourceMemory: resource.MustParse("1Gi"), corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: false,
		},
		{
			name: "CPU is not equal",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: resource.MustParse("2Gi"), corev1.ResourceMemory: resource.MustParse("4Gi"), corev1.ResourceCPU: resource.MustParse("8000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withStorageRequest("2Gi").withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: false,
		},
		{
			name: "Happy path",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: resource.MustParse("2Gi"), corev1.ResourceMemory: resource.MustParse("4Gi"), corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withStorageRequest("2Gi").withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: true,
		},
		{
			name: "CPU and Memory are equal, no storage",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("4Gi"), corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withMemoryRequest("4Gi").withCPURequest("2000m").build()},
			want: true,
		},
		{
			name: "Only memory",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("4Gi")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withMemoryRequest("4Gi").build()},
			want: true,
		},
		{
			name: "Only memory, not equal",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("8Gi")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withMemoryRequest("4Gi").build()},
			want: false,
		},
		{
			name: "Only CPU",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("2000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withCPURequest("2000m").build()},
			want: true,
		},
		{
			name: "Only CPU, not equal",
			fields: fields{
				Name:             "data-inject",
				NodeSetNodeCount: NodeSetNodeCountList{NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("4000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withCPURequest("2000m").build()},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ntr := NodeSetsResources{
				Name:             tt.fields.Name,
				NodeSetNodeCount: tt.fields.NodeSetNodeCount,
				NodeResources:    tt.fields.ResourcesSpecification,
			}
			got, err := ntr.Match(esv1.ElasticsearchContainerName, tt.args.nodeSet)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeSetsResources.Match() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NodeSetsResources.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// - NodeSet builder

type nodeSetBuilder struct {
	name                                      string
	count                                     int32
	memoryRequest, cpuRequest, storageRequest *resource.Quantity
}

func newNodeSetBuilder(name string, count int) *nodeSetBuilder {
	return &nodeSetBuilder{
		name:  name,
		count: int32(count),
	}
}

func (nsb *nodeSetBuilder) withMemoryRequest(qs string) *nodeSetBuilder {
	q := resource.MustParse(qs)
	nsb.memoryRequest = &q
	return nsb
}

func (nsb *nodeSetBuilder) withCPURequest(qs string) *nodeSetBuilder {
	q := resource.MustParse(qs)
	nsb.cpuRequest = &q
	return nsb
}

func (nsb *nodeSetBuilder) withStorageRequest(qs string) *nodeSetBuilder {
	q := resource.MustParse(qs)
	nsb.storageRequest = &q
	return nsb
}

func (nsb *nodeSetBuilder) build() esv1.NodeSet {
	nodeSet := esv1.NodeSet{
		Name:   nsb.name,
		Config: nil,
		Count:  nsb.count,
		PodTemplate: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: esv1.ElasticsearchContainerName,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{},
						},
					},
				},
			},
		},
	}

	// Set memory
	if nsb.memoryRequest != nil {
		nodeSet.PodTemplate.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory] = *nsb.memoryRequest
	}

	// Set CPU
	if nsb.cpuRequest != nil {
		nodeSet.PodTemplate.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = *nsb.cpuRequest
	}

	// Set storage
	if nsb.storageRequest != nil {
		storageRequest := corev1.ResourceList{}
		storageRequest[corev1.ResourceStorage] = *nsb.storageRequest
		nodeSet.VolumeClaimTemplates = append(nodeSet.VolumeClaimTemplates,
			corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: volume.ElasticsearchDataVolumeName,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: *nsb.storageRequest,
						},
					},
				},
			},
		)
	}
	return nodeSet
}

func float64ptr(f float64) *float64 {
	return &f
}
