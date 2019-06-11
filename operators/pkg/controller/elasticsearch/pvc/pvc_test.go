// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pvc

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	fastStorageClassname = "fast"
	sampleLabels1        = map[string]string{"label1": "value1", "label2": "value2"}
	sampleLabels2        = map[string]string{"label1": "value1", "label2": "value3"}
)

func newPVC(podName string, pvcName string, sourceLabels map[string]string,
	storageQty string, storageClassName *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:   pvcName,
			Labels: newPodLabel(podName, sourceLabels),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: storageClassName,
			Resources: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse(storageQty),
				},
			},
		},
	}
}

func deletePVC(pvc *corev1.PersistentVolumeClaim) *corev1.PersistentVolumeClaim {
	now := v1.Now()
	pvc.DeletionTimestamp = &now
	return pvc
}

func newPod(name string, sourceLabels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   name,
			Labels: newPodLabel(name, sourceLabels),
		},
	}
}

func newPodLabel(podName string, sourceLabels map[string]string) map[string]string {
	newMap := make(map[string]string)
	for key, value := range sourceLabels {
		newMap[key] = value
	}
	newMap[label.PodNameLabelName] = podName
	newMap[label.ClusterNameLabelName] = "elasticsearch-sample"
	return newMap
}

func withPVC(pod *corev1.Pod, volumeName string, claimName string) *corev1.Pod {
	pod.Spec.Volumes = []corev1.Volume{
		{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
					ReadOnly:  false,
				},
			},
		},
	}
	return pod
}

func TestFindOrphanedVolumeClaims(t *testing.T) {
	pvc1 := newPVC(
		"elasticsearch-sample-es-2l59jptdq6",
		"elasticsearch-sample-es-2l59jptdq6-"+volume.ElasticsearchDataVolumeName,
		sampleLabels1,
		"1Gi",
		nil,
	)
	pvc2 := newPVC(
		"elasticsearch-sample-es-6bw9qkw77k",
		"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
		sampleLabels1,
		"1Gi",
		nil,
	)
	pvc3 := newPVC(
		"elasticsearch-sample-es-6qg4hmd9dj",
		"elasticsearch-sample-es-6qg4hmd9dj-"+volume.ElasticsearchDataVolumeName,
		sampleLabels2,
		"1Gi",
		nil,
	)
	type args struct {
		initialObjects []runtime.Object
		es             v1alpha1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    *OrphanedPersistentVolumeClaims
		wantErr bool
	}{
		{
			name: "Simple",
			args: args{
				initialObjects: []runtime.Object{
					// create 1 Pod
					withPVC(
						newPod("elasticsearch-sample-es-2l59jptdq6", sampleLabels1),
						volume.ElasticsearchDataVolumeName,
						"elasticsearch-sample-es-2l59jptdq6-"+volume.ElasticsearchDataVolumeName,
					),
					// create 3 PVCs
					pvc1,
					pvc2,
					pvc3,
				},
				es: v1alpha1.Elasticsearch{
					ObjectMeta: v1.ObjectMeta{
						Name: "elasticsearch-sample",
					},
				},
			},
			want:    &OrphanedPersistentVolumeClaims{[]corev1.PersistentVolumeClaim{*pvc2, *pvc3}},
			wantErr: false,
		},
		{
			name: "With a deleted PVC",
			args: args{
				initialObjects: []runtime.Object{
					// create 1 Pod
					withPVC(
						newPod("elasticsearch-sample-es-2l59jptdq6", sampleLabels1),
						volume.ElasticsearchDataVolumeName,
						"elasticsearch-sample-es-2l59jptdq6-"+volume.ElasticsearchDataVolumeName,
					),
					// create 3 PVCs, but one of them is scheduled to be deleted
					pvc1,
					pvc2,
					deletePVC(pvc3.DeepCopy()),
				},
				es: v1alpha1.Elasticsearch{
					ObjectMeta: v1.ObjectMeta{
						Name: "elasticsearch-sample",
					},
				},
			},
			want:    &OrphanedPersistentVolumeClaims{[]corev1.PersistentVolumeClaim{*pvc2}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
			got, err := FindOrphanedVolumeClaims(fakeClient, tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindOrphanedVolumeClaims() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.ElementsMatch(t, got.orphanedPersistentVolumeClaims, tt.want.orphanedPersistentVolumeClaims) {
				t.Errorf("FindOrphanedVolumeClaims() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrphanedPersistentVolumeClaims_FindOrphanedVolumeClaim(t *testing.T) {
	type fields struct {
		orphanedPersistentVolumeClaims []corev1.PersistentVolumeClaim
	}
	type args struct {
		expectedLabels map[string]string
		claim          *corev1.PersistentVolumeClaim
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *corev1.PersistentVolumeClaim
	}{
		{
			name: "Simple test with a standard storage class and 1Gi of storage",
			fields: fields{
				[]corev1.PersistentVolumeClaim{
					*newPVC(
						"elasticsearch-sample-es-6bw9qkw77k",
						"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
						sampleLabels1,
						"1Gi",
						&standardStorageClassname,
					),
					*newPVC(
						"elasticsearch-sample-es-6qg4hmd9dj",
						"elasticsearch-sample-es-6qg4hmd9dj-"+volume.ElasticsearchDataVolumeName,
						sampleLabels1,
						"1Gi",
						&standardStorageClassname,
					),
				}},
			args: args{
				expectedLabels: newPodLabel("elasticsearch-sample-es-2l59jptdq6", sampleLabels1),
				claim: &corev1.PersistentVolumeClaim{
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			want: newPVC(
				"elasticsearch-sample-es-6bw9qkw77k",
				"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
				sampleLabels1,
				"1Gi",
				&standardStorageClassname,
			),
		}, {
			name: "Labels mismatch",
			fields: fields{
				[]corev1.PersistentVolumeClaim{
					*newPVC(
						"elasticsearch-sample-es-6bw9qkw77k",
						"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
						sampleLabels2,
						"1Gi",
						&standardStorageClassname,
					),
					*newPVC(
						"elasticsearch-sample-es-6qg4hmd9dj",
						"elasticsearch-sample-es-6qg4hmd9dj-"+volume.ElasticsearchDataVolumeName,
						sampleLabels2,
						"1Gi",
						&standardStorageClassname,
					),
				}},
			args: args{
				expectedLabels: newPodLabel("elasticsearch-sample-es-2l59jptdq6", sampleLabels1),
				claim: &corev1.PersistentVolumeClaim{
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			want: nil,
		}, {
			name: "Matching storage class",
			fields: fields{
				[]corev1.PersistentVolumeClaim{
					*newPVC(
						"elasticsearch-sample-es-6bw9qkw77k",
						"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
						sampleLabels1,
						"1Gi",
						&fastStorageClassname,
					),
					*newPVC(
						"elasticsearch-sample-es-6qg4hmd9dj",
						"elasticsearch-sample-es-6qg4hmd9dj-"+volume.ElasticsearchDataVolumeName,
						sampleLabels1,
						"1Gi",
						&fastStorageClassname,
					),
				}},
			args: args{
				expectedLabels: newPodLabel("elasticsearch-sample-es-2l59jptdq6", sampleLabels1),
				claim: &corev1.PersistentVolumeClaim{
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: &fastStorageClassname,
						Resources: corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								"storage": resource.MustParse("1024Mi"),
							},
						},
					},
				},
			},
			want: newPVC(
				"elasticsearch-sample-es-6bw9qkw77k",
				"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
				sampleLabels1,
				"1Gi",
				&fastStorageClassname,
			),
		},
		{
			name: "Storage class mismatch",
			fields: fields{
				[]corev1.PersistentVolumeClaim{
					*newPVC(
						"elasticsearch-sample-es-6bw9qkw77k",
						"elasticsearch-sample-es-6bw9qkw77k-"+volume.ElasticsearchDataVolumeName,
						sampleLabels1,
						"1Gi",
						&standardStorageClassname,
					),
					*newPVC(
						"elasticsearch-sample-es-6qg4hmd9dj",
						"elasticsearch-sample-es-6qg4hmd9dj-"+volume.ElasticsearchDataVolumeName,
						sampleLabels1,
						"1Gi",
						&standardStorageClassname,
					),
				}},
			args: args{
				expectedLabels: newPodLabel("elasticsearch-sample-es-2l59jptdq6", sampleLabels1),
				claim: &corev1.PersistentVolumeClaim{
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: &fastStorageClassname,
						Resources: corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &OrphanedPersistentVolumeClaims{
				orphanedPersistentVolumeClaims: tt.fields.orphanedPersistentVolumeClaims,
			}
			if got := o.GetOrphanedVolumeClaim(tt.args.expectedLabels, tt.args.claim); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OrphanedPersistentVolumeClaims.GetOrphanedVolumeClaim() = %v, want %v", got, tt.want)
			}
		})
	}
}
