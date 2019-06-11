// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// NewConfigMapVolume creates a new ConfigMapVolume struct
func NewConfigMapVolume(configMapName, name, mountPath string) ConfigMapVolume {
	return NewConfigMapVolumeWithMode(configMapName, name, mountPath, corev1.ConfigMapVolumeSourceDefaultMode)
}

// NewConfigMapVolumeWithMode creates a new ConfigMapVolume struct with default mode
func NewConfigMapVolumeWithMode(configMapName, name, mountPath string, defaultMode int32) ConfigMapVolume {
	return ConfigMapVolume{
		configMapName: configMapName,
		name:          name,
		mountPath:     mountPath,
		defaultMode:   defaultMode,
	}
}

// ConfigMapVolume defines a volume to expose a configmap
type ConfigMapVolume struct {
	configMapName string
	name          string
	mountPath     string
	items         []corev1.KeyToPath
	defaultMode   int32
}

// VolumeMount returns the k8s volume mount.
func (cm ConfigMapVolume) VolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      cm.name,
		MountPath: cm.mountPath,
		ReadOnly:  true,
	}
}

// Volume returns the k8s volume.
func (cm ConfigMapVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: cm.name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cm.configMapName,
				},
				Items:       cm.items,
				Optional:    &defaultOptional,
				DefaultMode: &cm.defaultMode,
			},
		},
	}
}

// Name returns the name of the volume
func (cm ConfigMapVolume) Name() string {
	return cm.name
}

var _ VolumeLike = ConfigMapVolume{}
