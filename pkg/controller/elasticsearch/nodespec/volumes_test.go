// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	esvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

// Test_BuildVolumes_DataVolumeMountPath tests that the elasticsearch-data volumeMount is always set.
func Test_BuildVolumes_DataVolumeMountPath(t *testing.T) {
	hostPathType := corev1.HostPathDirectoryOrCreate

	tt := []struct {
		name     string
		nodeSpec esv1.NodeSet
		want     []corev1.ContainerPort
	}{
		{
			name: "with eck default data PVC",
			nodeSpec: esv1.NodeSet{
				VolumeClaimTemplates: esvolume.DefaultVolumeClaimTemplates,
			},
		},
		{
			name: "with user provided data PVC",
			nodeSpec: esv1.NodeSet{
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("42Ti"),
							},
						},
					},
				}},
			},
		},
		{
			name: "with user provided data empty volume",
			nodeSpec: esv1.NodeSet{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "elasticsearch-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							}},
						},
					},
				},
			},
		},
		{
			name: "with user provided data hostpath volume",
			nodeSpec: esv1.NodeSet{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "elasticsearch-data",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/mnt/data",
									Type: &hostPathType,
								},
							},
						}},
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			_, volumeMounts := buildVolumes("esname", version.MustParse("8.8.0"), tc.nodeSpec, nil, volume.DownwardAPI{}, []volume.VolumeLike{})
			assert.True(t, contains(volumeMounts, "elasticsearch-data", "/usr/share/elasticsearch/data"))
		})
	}
}

func contains(volumeMounts []corev1.VolumeMount, volumeMountName, volumeMountPath string) bool {
	for _, vm := range volumeMounts {
		if vm.Name == volumeMountName && vm.MountPath == volumeMountPath {
			return true
		}
	}
	return false
}
