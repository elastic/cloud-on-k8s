// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"

)

var (
	// ConfigSharedVolume contains the Logstash config/ directory, it contains the contents of config from the docker container
	ConfigSharedVolume = volume.SharedVolume{
		VolumeName:             ConfigVolumeName,
		InitContainerMountPath: InitContainerConfigVolumeMountPath,
		ContainerMountPath:     ConfigMountPath,
	}

	DefaultPersistentVolumeSize = resource.MustParse("1Gi")

	// DefaultDataVolumeClaim is the default data volume claim for Logstash pods.
	// We default to a 1GB persistent volume, using the default storage class.
	DefaultDataVolumeClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: LogstashDataVolumeName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
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

	// DefaultLogsVolume is the default EmptyDir logs volume for Logstash pods.
	DefaultLogsVolume = corev1.Volume{
		Name: LogstashLogsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	// DefaultLogsVolumeMount is the default logs volume mount for the Logstash container.
	DefaultLogsVolumeMount = corev1.VolumeMount{
		Name:      LogstashLogsVolumeName,
		MountPath: LogstashLogsMountPath,
	}
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

// AppendDefaultLogVolumeMount appends a volume mount for the default log volume if the slice of volumes contains the default log volume.
func AppendDefaultLogVolumeMount(mounts []corev1.VolumeMount, volumes []corev1.Volume) []corev1.VolumeMount {
	for _, v := range volumes {
		if v.Name == LogstashLogsVolumeName {
			return append(mounts, DefaultLogsVolumeMount)
		}
	}
	return mounts
}

// AppendDefaultPVCs appends defaults PVCs to a set of existing ones.
//
// The default PVCs are not appended if:
// - a Volume with the same .Name is found in podSpec.Volumes, and that volume is not a PVC volume
func AppendDefaultPVCs(
	existing []corev1.PersistentVolumeClaim,
	podSpec corev1.PodSpec,
	defaults ...corev1.PersistentVolumeClaim,
) []corev1.PersistentVolumeClaim {
	// create a set of volume names that are not PVC-volumes
	nonPVCvolumes := set.Make()

	for _, volume := range podSpec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// this volume is not a PVC
			nonPVCvolumes.Add(volume.Name)
		}
	}

	for _, defaultPVC := range defaults {
		if nonPVCvolumes.Has(defaultPVC.Name) {
			continue
		}
		existing = append(existing, defaultPVC)
	}
	return existing
}
