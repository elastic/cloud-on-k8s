// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import corev1 "k8s.io/api/core/v1"

// SharedVolume between the init container and the ES container.
type SharedVolume struct {
	Name                   string // Volume name
	InitContainerMountPath string // Mount path in the init container
	EsContainerMountPath   string // Mount path in the Elasticsearch container
}

func (v SharedVolume) InitContainerVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		MountPath: v.InitContainerMountPath,
		Name:      v.Name,
	}
}

func (v SharedVolume) EsContainerVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		MountPath: v.EsContainerMountPath,
		Name:      v.Name,
	}
}

func (v SharedVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: v.Name,
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

func (v SharedVolumeArray) EsContainerVolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, len(v.Array))
	for i, v := range v.Array {
		mounts[i] = v.EsContainerVolumeMount()
	}
	return mounts
}

func (v SharedVolumeArray) Volumes() []corev1.Volume {
	volumes := make([]corev1.Volume, len(v.Array))
	for i, v := range v.Array {
		volumes[i] = corev1.Volume{
			Name: v.Name,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}
	return volumes
}
