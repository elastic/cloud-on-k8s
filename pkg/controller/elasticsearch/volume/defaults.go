// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	DefaultPersistentVolumeSize = resource.MustParse("1Gi")

	// DefaultDataVolumeClaim is the default data volume claim for Elasticsearch pods.
	// We default to a 1GB persistent volume, using the default storage class.
	DefaultDataVolumeClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: ElasticsearchDataVolumeName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: DefaultPersistentVolumeSize,
				},
			},
		},
	}
	DefaultDataVolumeMount = corev1.VolumeMount{
		Name:      ElasticsearchDataVolumeName,
		MountPath: ElasticsearchDataMountPath,
	}

	// DefaultVolumeClaimTemplates is the default volume claim templates for Elasticsearch pods
	DefaultVolumeClaimTemplates = []corev1.PersistentVolumeClaim{DefaultDataVolumeClaim}

	// DefaultLogsVolume is the default EmptyDir logs volume for Elasticsearch pods.
	DefaultLogsVolume = corev1.Volume{
		Name: ElasticsearchLogsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	// DefaultLogsVolumeMount is the default logs volume mount for the Elasticsearch container.
	DefaultLogsVolumeMount = corev1.VolumeMount{
		Name:      ElasticsearchLogsVolumeName,
		MountPath: ElasticsearchLogsMountPath,
	}
)

// AppendDefaultDataVolumeMount appends a volume mount for the default data volume if the slice of volumes contains the default data volume.
func AppendDefaultDataVolumeMount(mounts []corev1.VolumeMount, volumes []corev1.Volume) []corev1.VolumeMount {
	for _, v := range volumes {
		if v.Name == ElasticsearchDataVolumeName {
			return append(mounts, DefaultDataVolumeMount)
		}
	}
	return mounts
}
