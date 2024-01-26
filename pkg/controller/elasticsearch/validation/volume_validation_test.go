// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	sampleStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "sample-sc"}}

	sampleClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To[string](sampleStorageClass.Name),
			Resources: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
	sampleClaim2 = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim-2"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To[string](sampleStorageClass.Name),
			Resources: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
)

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func Test_validPVCModification(t *testing.T) {
	es := func(nodeSets []esv1.NodeSet) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
			Spec:       esv1.ElasticsearchSpec{NodeSets: nodeSets},
		}
	}
	type args struct {
		current              esv1.Elasticsearch
		proposed             esv1.Elasticsearch
		k8sClient            k8s.Client
		validateStorageClass bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no changes in the claims: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "new nodeSet: ok",
			args: args{
				current: es([]esv1.NodeSet{}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient:            k8s.NewFakeClient(),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "statefulSet does not exist: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.NewFakeClient(),
			},
			wantErr: false,
		},
		{
			name: "modified claims (one less) in the proposed Elasticsearch: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "modified claims (new name) in the proposed Elasticsearch: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage decrease in the proposed elasticsearch vs. existing statefulset: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")}}, // decrease
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage decrease in the proposed elasticsearch vs. current elasticsearch, but matches current sset: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")}}, // revert to previous size
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, withStorageReq(sampleClaim2, "0.5Gi"),
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validPVCModification(context.Background(), tt.args.current, tt.args.proposed, tt.args.k8sClient, tt.args.validateStorageClass)
			if tt.wantErr {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}

func Test_validPVCNaming(t *testing.T) {
	esFixture := func() esv1.Elasticsearch {
		return esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{
			{Name: "default"},
		}}}
	}
	esWithClaim := func(claimName string, es esv1.Elasticsearch) esv1.Elasticsearch {
		es.Spec.NodeSets[0].VolumeClaimTemplates = append(es.Spec.NodeSets[0].VolumeClaimTemplates, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: claimName},
		})
		return es
	}
	esWithVolumeMount := func(mountName string, es esv1.Elasticsearch) esv1.Elasticsearch {
		if es.Spec.NodeSets[0].PodTemplate.Spec.Containers == nil {
			es.Spec.NodeSets[0].PodTemplate.Spec.Containers = []corev1.Container{
				{Name: "elasticsearch"},
			}
		}
		es.Spec.NodeSets[0].PodTemplate.Spec.Containers[0].VolumeMounts = append(
			es.Spec.NodeSets[0].PodTemplate.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      mountName,
				MountPath: "/something/we/cannot/check/as/it/is/customizable/in/elasticsearch.yml",
			},
		)
		return es
	}
	esWithSidecar := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name: "default",
					PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
						{
							Name: "sidecar",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "my-data"},
							},
						}}}},
				},
			},
		},
	}
	tests := []struct {
		name    string
		es      esv1.Elasticsearch
		wantErr bool
	}{
		{
			name:    "no claims is OK",
			es:      esFixture(),
			wantErr: false,
		},
		{
			name:    "default volume claim name is OK",
			es:      esWithClaim("elasticsearch-data", esFixture()),
			wantErr: false,
		},
		{
			name:    "custom claim name not mounted is NOK",
			es:      esWithClaim("my-data", esFixture()),
			wantErr: true,
		},
		{
			name:    "custom claim name but mounted is OK",
			es:      esWithVolumeMount("my-data", esWithClaim("my-data", esFixture())),
			wantErr: false,
		},
		{
			name:    "multiple custom claims but one not mounted is NOK",
			es:      esWithVolumeMount("my-data", esWithClaim("yet-another", esWithClaim("my-data", esFixture()))),
			wantErr: true,
		},
		{
			name:    "multiple custom claims is OK",
			es:      esWithVolumeMount("yet-another", esWithVolumeMount("my-data", esWithClaim("yet-another", esWithClaim("my-data", esFixture())))),
			wantErr: false,
		},
		{
			name: "custom claims for sidecars if all are mounted is OK",
			// this example has no valid data volume but if we want to allow data path customization there is no easy way to validate that
			es:      esWithClaim("my-data", esWithSidecar),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validPVCNaming(tt.es)
			if tt.wantErr {
				require.NotEmpty(t, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}
