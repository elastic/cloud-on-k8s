// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

func TestReconcileElasticsearch(t *testing.T) {
	tests := []struct {
		name                  string
		initialES             esv1.Elasticsearch
		nextClusterResources  v1alpha1.ClusterResources
		wantNodeSetCount      int32
		wantCPURequest        string
		wantMemoryRequest     string
		wantCPULimit          string
		wantMemoryLimit       string
		wantStorageRequest    string
		wantPodTemplateCPU    string
		wantPodTemplateMemory string
	}{
		{
			name: "updates nodeSet resources and keeps pod template resources untouched",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "hot",
							Count: 1,
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("500m")),
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
								Limits: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("1000m")),
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("750m"),
													corev1.ResourceMemory: resource.MustParse("1500Mi"),
												},
											},
										},
									},
								},
							},
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{Name: volume.ElasticsearchDataVolumeName},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.VolumeResourceRequirements{
											Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-hot",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "hot", NodeCount: 3},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:     resource.MustParse("2000m"),
							corev1.ResourceMemory:  resource.MustParse("4Gi"),
							corev1.ResourceStorage: resource.MustParse("8Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("3000m"),
							corev1.ResourceMemory: resource.MustParse("6Gi"),
						},
					},
				},
			},
			wantNodeSetCount:      3,
			wantCPURequest:        "2000m",
			wantMemoryRequest:     "4Gi",
			wantCPULimit:          "3000m",
			wantMemoryLimit:       "6Gi",
			wantStorageRequest:    "8Gi",
			wantPodTemplateCPU:    "750m",
			wantPodTemplateMemory: "1500Mi",
		},
		{
			name: "missing cpu recommendations preserves existing cpu values",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "warm",
							Count: 2,
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("800m")),
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
								Limits: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("1200m")),
									Memory: quantityPtr(resource.MustParse("3Gi")),
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("900m"),
													corev1.ResourceMemory: resource.MustParse("1Gi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-warm",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "warm", NodeCount: 4},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("5Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("7Gi"),
						},
					},
				},
			},
			wantNodeSetCount:      4,
			wantCPURequest:        "800m",
			wantMemoryRequest:     "5Gi",
			wantCPULimit:          "1200m",
			wantMemoryLimit:       "7Gi",
			wantPodTemplateCPU:    "900m",
			wantPodTemplateMemory: "1Gi",
		},
		{
			name: "missing cpu recommendations preserves nil cpu values",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "cold",
							Count: 1,
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									Memory: quantityPtr(resource.MustParse("1Gi")),
								},
								Limits: commonv1.ResourceAllocations{
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("700m"),
													corev1.ResourceMemory: resource.MustParse("1400Mi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-cold",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "cold", NodeCount: 2},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("3Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			wantNodeSetCount:      2,
			wantCPURequest:        "",
			wantMemoryRequest:     "3Gi",
			wantCPULimit:          "",
			wantMemoryLimit:       "4Gi",
			wantPodTemplateCPU:    "700m",
			wantPodTemplateMemory: "1400Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.initialES.DeepCopy()

			err := reconcileElasticsearch(logr.Discard(), es, tt.nextClusterResources)
			require.NoError(t, err)
			require.Len(t, es.Spec.NodeSets, 1)

			nodeSet := es.Spec.NodeSets[0]
			assert.Equal(t, tt.wantNodeSetCount, nodeSet.Count)
			assertQuantityPointerEqual(t, tt.wantCPURequest, nodeSet.Resources.Requests.CPU)
			assertQuantityPointerEqual(t, tt.wantMemoryRequest, nodeSet.Resources.Requests.Memory)
			assertQuantityPointerEqual(t, tt.wantCPULimit, nodeSet.Resources.Limits.CPU)
			assertQuantityPointerEqual(t, tt.wantMemoryLimit, nodeSet.Resources.Limits.Memory)

			if tt.wantStorageRequest != "" {
				require.Len(t, nodeSet.VolumeClaimTemplates, 1)
				assert.True(
					t,
					resource.MustParse(tt.wantStorageRequest).Equal(nodeSet.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]),
				)
			}

			mainContainer := getMainContainer(nodeSet)
			require.NotNil(t, mainContainer)
			assert.True(t, resource.MustParse(tt.wantPodTemplateCPU).Equal(mainContainer.Resources.Requests[corev1.ResourceCPU]))
			assert.True(t, resource.MustParse(tt.wantPodTemplateMemory).Equal(mainContainer.Resources.Requests[corev1.ResourceMemory]))
		})
	}
}

// getMainContainer returns the Elasticsearch main container from a NodeSet pod template.
func getMainContainer(nodeSet esv1.NodeSet) *corev1.Container {
	for i := range nodeSet.PodTemplate.Spec.Containers {
		container := nodeSet.PodTemplate.Spec.Containers[i]
		if container.Name == esv1.ElasticsearchContainerName {
			return &container
		}
	}
	return nil
}

// assertQuantityPointerEqual compares a quantity pointer value with an expected quantity string.
// An empty expected string asserts that the pointer is nil.
func assertQuantityPointerEqual(t *testing.T, expected string, current *resource.Quantity) {
	t.Helper()
	if expected == "" {
		assert.Nil(t, current)
		return
	}
	require.NotNil(t, current)
	assert.True(t, resource.MustParse(expected).Equal(*current))
}

// quantityPtr returns a pointer to a copy of the provided quantity value.
func quantityPtr(quantity resource.Quantity) *resource.Quantity {
	q := quantity
	return &q
}
