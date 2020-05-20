// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// NewPersistentVolumeClaim creates a new PersistentVolumeClaim struct
func NewPersistentVolumeClaim(name, mountPath string) HostVolume {
	return HostVolume{
		name:      name,
		mountPath: mountPath,
	}
}

// PersistentVolumeClaim defines a persistent volume claim
type PersistentVolumeClaim struct {
	name      string
	mountPath string
}

// VolumeMount returns the k8s volume mount.
func (pvc PersistentVolumeClaim) VolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      pvc.name,
		MountPath: pvc.mountPath,
	}
}

// Volume returns the k8s volume.
func (pvc PersistentVolumeClaim) Volume() corev1.Volume {
	return corev1.Volume{
		Name: pvc.name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				// actual claim name will be resolved and fixed right before pod creation
				ClaimName: "claim-name-placeholder",
			},
		},
	}
}

// Name returns the name of the volume
func (pvc PersistentVolumeClaim) Name() string {
	return pvc.name
}

var _ VolumeLike = PersistentVolumeClaim{}
