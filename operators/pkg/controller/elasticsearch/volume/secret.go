// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// SecretVolume captures a subset of data of the k8s secrete volume/mount type.
type SecretVolume struct {
	name       string
	mountPath  string
	secretName string
	items      []corev1.KeyToPath
}

// NewSecretVolume creates a new SecretVolume with default mount path.
func NewSecretVolume(secretName string, name string) SecretVolume {
	return NewSecretVolumeWithMountPath(secretName, name, DefaultSecretMountPath)
}

// NewSecretVolumeWithMountPath creates a new SecretVolume
func NewSecretVolumeWithMountPath(secretName string, name string, mountPath string) SecretVolume {
	return SecretVolume{
		name:       name,
		mountPath:  mountPath,
		secretName: secretName,
	}
}

// NewSelectiveSecretVolumeWithMountPath creates a new SecretVolume that projects only the specified secrets into the file system.
func NewSelectiveSecretVolumeWithMountPath(secretName string, name string, mountPath string, projectedSecrets []string) SecretVolume {
	var keyToPaths []corev1.KeyToPath
	for _, s := range projectedSecrets {
		keyToPaths = append(keyToPaths, corev1.KeyToPath{
			Key:  s,
			Path: s,
		})
	}
	return SecretVolume{
		name:       name,
		mountPath:  mountPath,
		secretName: secretName,
		items:      keyToPaths,
	}
}

// VolumeMount returns the k8s volume mount.
func (sv SecretVolume) VolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      sv.name,
		MountPath: sv.mountPath,
		ReadOnly:  true,
	}
}

// Volume returns the k8s volume.
func (sv SecretVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: sv.name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: sv.secretName,
				Items:      sv.items,
				Optional:   &defaultOptional,
			},
		},
	}
}

// Name returns the name of the volume
func (sv SecretVolume) Name() string {
	return sv.name
}

var _ VolumeLike = SecretVolume{}
