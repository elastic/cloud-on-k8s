// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
)

var (
	// ConfigSharedVolume contains the Logstash config/ directory, it contains the contents of config from the docker container
	ConfigSharedVolume = volume.SharedVolume{
		VolumeName:             ConfigVolumeName,
		InitContainerMountPath: InitContainerConfigVolumeMountPath,
		ContainerMountPath:     ConfigMountPath,
	}

	DefaultPersistentVolumeSize = resource.MustParse("1.5Gi")

	// DefaultDataVolumeClaim is the default data volume claim for Logstash pods.
	// We default to a 1.5Gi persistent volume, using the default storage class.
	DefaultDataVolumeClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: LogstashDataVolumeName,
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
		Name:      LogstashDataVolumeName,
		MountPath: LogstashDataMountPath,
	}

	// DefaultVolumeClaimTemplates is the default volume claim templates for Logstash pods
	DefaultVolumeClaimTemplates = []corev1.PersistentVolumeClaim{DefaultDataVolumeClaim}

	DefaultLogsVolume = volume.NewEmptyDirVolume(
		LogstashLogsVolumeName,
		LogstashLogsMountPath,
	)
)

// ConfigVolume returns a SecretVolume to hold the Logstash config of the given Logstash resource.
func ConfigVolume(ls logstashv1alpha1.Logstash) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		logstashv1alpha1.ConfigSecretName(ls.Name),
		InternalConfigVolumeName,
		InternalConfigVolumeMountPath,
	)
}

// PipelineVolume returns a SecretVolume to hold the Logstash config of the given Logstash resource.
func PipelineVolume(ls logstashv1alpha1.Logstash) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		logstashv1alpha1.PipelineSecretName(ls.Name),
		InternalPipelineVolumeName,
		InternalPipelineVolumeMountPath,
	)
}

// AppendDefaultDataVolumeMount appends a volume mount for the default data volume if the slice of volumes contains the default data volume.
func AppendDefaultDataVolumeMount(mounts []corev1.VolumeMount, volumes []corev1.Volume) []corev1.VolumeMount {
	for _, v := range volumes {
		if v.Name == LogstashDataVolumeName {
			return append(mounts, DefaultDataVolumeMount)
		}
	}
	return mounts
}
