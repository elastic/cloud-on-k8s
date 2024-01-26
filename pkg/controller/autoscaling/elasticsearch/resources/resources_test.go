// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

func TestNodeSetsResources_Match(t *testing.T) {
	type fields struct {
		Name                   string
		NodeSetNodeCount       v1alpha1.NodeSetNodeCountList
		ResourcesSpecification v1alpha1.NodeResources
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 6}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
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
				NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{v1alpha1.NodeSetNodeCount{Name: "nodeset-1", NodeCount: 3}, v1alpha1.NodeSetNodeCount{Name: "nodeset-2", NodeCount: 5}},
				ResourcesSpecification: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceCPU: resource.MustParse("4000m")},
				},
			},
			args: args{nodeSet: newNodeSetBuilder("nodeset-2", 5).withCPURequest("2000m").build()},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ntr := v1alpha1.NodeSetsResources{
				Name:             tt.fields.Name,
				NodeSetNodeCount: tt.fields.NodeSetNodeCount,
				NodeResources:    tt.fields.ResourcesSpecification,
			}
			got, err := Match(ntr, esv1.ElasticsearchContainerName, tt.args.nodeSet)
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
					Resources: corev1.VolumeResourceRequirements{
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
