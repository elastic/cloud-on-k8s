// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import corev1 "k8s.io/api/core/v1"

// SharedVolume between the init container and the main container.
type SharedVolume struct {
	VolumeName             string // Volume name
	InitContainerMountPath string // Mount path in the init container
	ContainerMountPath     string // Mount path in the main container (e.g. Elasticsearch or Kibana)
}

func (v SharedVolume) InitContainerVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		MountPath: v.InitContainerMountPath,
		Name:      v.VolumeName,
	}
}

func (v SharedVolume) Name() string {
	return v.VolumeName
}

func (v SharedVolume) VolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		MountPath: v.ContainerMountPath,
		Name:      v.VolumeName,
	}
}

func (v SharedVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: v.VolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// SharedVolumes represents a list of SharedVolume
type SharedVolumeArray struct {
	Array []SharedVolume
}

func (v SharedVolumeArray) InitContainerVolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, len(v.Array))
	for i, v := range v.Array {
		mounts[i] = v.InitContainerVolumeMount()
	}
	return mounts
}

func (v SharedVolumeArray) ContainerVolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, len(v.Array))
	for i, v := range v.Array {
		mounts[i] = v.VolumeMount()
	}
	return mounts
}

func (v SharedVolumeArray) Volumes() []corev1.Volume {
	volumes := make([]corev1.Volume, len(v.Array))
	for i, v := range v.Array {
		volumes[i] = corev1.Volume{
			Name: v.VolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}
	return volumes
}
