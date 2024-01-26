// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// SecretVolume captures a subset of data of the k8s secret volume/mount type.
type SecretVolume struct {
	name        string
	mountPath   string
	secretName  string
	items       []corev1.KeyToPath
	subPath     string
	defaultMode *int32
}

// NewSecretVolumeWithMountPath creates a new SecretVolume
func NewSecretVolumeWithMountPath(secretName, name, mountPath string) SecretVolume {
	return SecretVolume{
		name:       name,
		mountPath:  mountPath,
		secretName: secretName,
	}
}

// NewSecretVolume creates a new SecretVolume
func NewSecretVolume(secretName, name, mountPath, subPath string, defaultMode int32) SecretVolume {
	return SecretVolume{
		name:        name,
		mountPath:   mountPath,
		secretName:  secretName,
		subPath:     subPath,
		defaultMode: ptr.To[int32](defaultMode),
	}
}

// NewSelectiveSecretVolumeWithMountPath creates a new SecretVolume that projects only the specified secrets into the file system.
func NewSelectiveSecretVolumeWithMountPath(secretName, name, mountPath string, projectedSecrets []string) SecretVolume {
	keyToPaths := make([]corev1.KeyToPath, len(projectedSecrets))
	for i, s := range projectedSecrets {
		keyToPaths[i] = corev1.KeyToPath{
			Key:  s,
			Path: s,
		}
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
		SubPath:   sv.subPath,
	}
}

// Volume returns the k8s volume.
func (sv SecretVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: sv.name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  sv.secretName,
				Items:       sv.items,
				Optional:    &defaultOptional,
				DefaultMode: sv.defaultMode,
			},
		},
	}
}

// Name returns the name of the volume
func (sv SecretVolume) Name() string {
	return sv.name
}

var _ VolumeLike = SecretVolume{}
