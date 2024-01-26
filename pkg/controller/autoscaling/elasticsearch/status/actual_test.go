// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package status

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestNodeSetsResourcesResourcesFromStatefulSets(t *testing.T) {
	type args struct {
		statefulSets          []client.Object
		es                    esv1.Elasticsearch
		autoscalingPolicySpec v1alpha1.AutoscalingPolicySpec
		nodeSets              []string
	}
	tests := []struct {
		name                  string
		args                  args
		wantNodeSetsResources *v1alpha1.NodeSetsResources
		wantErr               bool
	}{
		{
			name: "No existing StatefulSet",
			args: args{
				statefulSets: []client.Object{ /* no existing StatefulSet */ },
				es:           esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "esname", Namespace: "esns"}},
				autoscalingPolicySpec: v1alpha1.AutoscalingPolicySpec{
					NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "aspec"},
					AutoscalingResources:   v1alpha1.AutoscalingResources{StorageRange: &v1alpha1.QuantityRange{Min: resource.MustParse("7Gi"), Max: resource.MustParse("50Gi")}}},
				nodeSets: []string{"nodeset-1", "nodeset-2"},
			},
			wantNodeSetsResources: nil,
		},
		{
			name: "Has existing resources only with storage",
			args: args{
				statefulSets: []client.Object{
					buildStatefulSet(
						"nodeset-1",
						3,
						map[string]corev1.ResourceRequirements{},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("5Gi")},
					),
					buildStatefulSet(
						"nodeset-2",
						2,
						map[string]corev1.ResourceRequirements{},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("10Gi")},
					),
				},
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "esname", Namespace: "esns"}},
				autoscalingPolicySpec: v1alpha1.AutoscalingPolicySpec{
					NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "aspec"},
					AutoscalingResources:   v1alpha1.AutoscalingResources{StorageRange: &v1alpha1.QuantityRange{Min: resource.MustParse("7Gi"), Max: resource.MustParse("50Gi")}}},
				nodeSets: []string{"nodeset-1", "nodeset-2"},
			},
			wantNodeSetsResources: &v1alpha1.NodeSetsResources{
				Name:             "aspec",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "nodeset-1", NodeCount: 3}, {Name: "nodeset-2", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		},
		{
			name: "Has existing resources, happy path",
			args: args{
				statefulSets: []client.Object{
					buildStatefulSet(
						"nodeset-1",
						3,
						map[string]corev1.ResourceRequirements{"elasticsearch": {
							Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("32Gi")},
						}},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("5Gi")},
					),
					buildStatefulSet(
						"nodeset-2",
						2,
						map[string]corev1.ResourceRequirements{"elasticsearch": {
							Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("24Gi")},
						}},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("10Gi")},
					),
				},
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "esname", Namespace: "esns"}},
				autoscalingPolicySpec: v1alpha1.AutoscalingPolicySpec{
					NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "aspec"},
					AutoscalingResources: v1alpha1.AutoscalingResources{
						MemoryRange:  &v1alpha1.QuantityRange{Min: resource.MustParse("12Gi"), Max: resource.MustParse("64Gi")},
						StorageRange: &v1alpha1.QuantityRange{Min: resource.MustParse("7Gi"), Max: resource.MustParse("50Gi")},
					},
				},
				nodeSets: []string{"nodeset-1", "nodeset-2"},
			},
			wantNodeSetsResources: &v1alpha1.NodeSetsResources{
				Name:             "aspec",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "nodeset-1", NodeCount: 3}, {Name: "nodeset-2", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory:  resource.MustParse("32Gi"),
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		},
		{
			name: "No volume claim",
			args: args{
				statefulSets: []client.Object{
					buildStatefulSet(
						"nodeset-1",
						3,
						map[string]corev1.ResourceRequirements{"elasticsearch": {
							Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("32Gi")},
						}},
						map[string]resource.Quantity{},
					),
					buildStatefulSet(
						"nodeset-2",
						2,
						map[string]corev1.ResourceRequirements{"elasticsearch": {
							Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("24Gi")},
						}},
						map[string]resource.Quantity{},
					),
				},
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "esname", Namespace: "esns"}},
				autoscalingPolicySpec: v1alpha1.AutoscalingPolicySpec{
					NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "aspec"},
					AutoscalingResources: v1alpha1.AutoscalingResources{
						MemoryRange:  &v1alpha1.QuantityRange{Min: resource.MustParse("12Gi"), Max: resource.MustParse("64Gi")},
						StorageRange: &v1alpha1.QuantityRange{Min: resource.MustParse("7Gi"), Max: resource.MustParse("50Gi")},
					},
				},
				nodeSets: []string{"nodeset-1", "nodeset-2"},
			},
			wantNodeSetsResources: &v1alpha1.NodeSetsResources{
				Name:             "aspec",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "nodeset-1", NodeCount: 3}, {Name: "nodeset-2", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("32Gi"),
					},
				},
			},
		},
		{
			name: "Several volume claims",
			args: args{
				statefulSets: []client.Object{
					buildStatefulSet(
						"nodeset-1",
						3,
						map[string]corev1.ResourceRequirements{},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("5Gi")},
					),
					buildStatefulSet(
						"nodeset-2",
						2,
						map[string]corev1.ResourceRequirements{},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("10Gi"), "other": resource.MustParse("10Gi")},
					),
				},
				es:                    esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "esname", Namespace: "esns"}},
				autoscalingPolicySpec: v1alpha1.AutoscalingPolicySpec{NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "aspec"}},
				nodeSets:              []string{"nodeset-1", "nodeset-2"},
			},
			wantErr:               true,
			wantNodeSetsResources: nil,
		},
		{
			name: "Not the default volume claims",
			args: args{
				statefulSets: []client.Object{
					buildStatefulSet(
						"nodeset-1",
						3,
						map[string]corev1.ResourceRequirements{},
						map[string]resource.Quantity{volume.ElasticsearchDataVolumeName: resource.MustParse("5Gi")},
					),
					buildStatefulSet(
						"nodeset-2",
						2,
						map[string]corev1.ResourceRequirements{},
						map[string]resource.Quantity{"other": resource.MustParse("10Gi")},
					),
				},
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "esname", Namespace: "esns"}},
				autoscalingPolicySpec: v1alpha1.AutoscalingPolicySpec{
					NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "aspec"},
					AutoscalingResources: v1alpha1.AutoscalingResources{
						MemoryRange:  &v1alpha1.QuantityRange{Min: resource.MustParse("12Gi"), Max: resource.MustParse("64Gi")},
						StorageRange: &v1alpha1.QuantityRange{Min: resource.MustParse("7Gi"), Max: resource.MustParse("50Gi")},
					},
				},
				nodeSets: []string{"nodeset-1", "nodeset-2"},
			},
			wantErr: false,
			wantNodeSetsResources: &v1alpha1.NodeSetsResources{
				Name:             "aspec",
				NodeSetNodeCount: []v1alpha1.NodeSetNodeCount{{Name: "nodeset-1", NodeCount: 3}, {Name: "nodeset-2", NodeCount: 2}},
				NodeResources: v1alpha1.NodeResources{
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.args.statefulSets...)
			got, err := nodeSetsResourcesResourcesFromStatefulSets(c, tt.args.es, tt.args.autoscalingPolicySpec, tt.args.nodeSets)
			if (err != nil) != tt.wantErr {
				t.Errorf("nodeSetsResourcesResourcesFromStatefulSets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.wantNodeSetsResources) {
				t.Errorf("nodeSetsResourcesResourcesFromStatefulSets() got = %v, want %v", got, tt.wantNodeSetsResources)
			}
		})
	}
}

func buildStatefulSet(
	nodeSetName string, replicas int,
	containersResources map[string]corev1.ResourceRequirements,
	volumeClaimTemplates map[string]resource.Quantity,
) *appsv1.StatefulSet {
	statefulSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.StatefulSet("esname", nodeSetName),
			Namespace: "esns",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32ptr(replicas),
		},
	}

	// Add volumes
	for volumeName, volumeRequest := range volumeClaimTemplates {
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: volumeName},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceStorage: volumeRequest},
				},
			},
		}
		statefulSet.Spec.VolumeClaimTemplates = append(statefulSet.Spec.VolumeClaimTemplates, pvc)
	}

	// Add containers
	for containerName, containerResources := range containersResources {
		container := corev1.Container{
			Name:      containerName,
			Resources: containerResources,
		}
		statefulSet.Spec.Template.Spec.Containers = append(statefulSet.Spec.Template.Spec.Containers, container)
	}

	return &statefulSet
}

func int32ptr(i int) *int32 {
	v := int32(i)
	return &v
}
