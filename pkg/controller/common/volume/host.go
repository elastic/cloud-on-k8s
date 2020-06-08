// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// NewReadOnlyHostVolume creates a new HostVolume struct.
func NewReadOnlyHostVolume(name, hostPath, mountPath string) HostVolume {
	return NewHostVolume(name, hostPath, mountPath, false, corev1.HostPathUnset)
}

// NewHostVolume creates a new HostVolume struct with default mode
func NewHostVolume(name, hostPath, mountPath string, readOnly bool, hostPathType corev1.HostPathType) HostVolume {
	return HostVolume{
		name:         name,
		hostPath:     hostPath,
		mountPath:    mountPath,
		readOnly:     readOnly,
		hostPathType: &hostPathType,
	}
}

// HostVolume defines a volume to expose a host path
type HostVolume struct {
	name         string
	hostPath     string
	mountPath    string
	readOnly     bool
	hostPathType *corev1.HostPathType
}

// VolumeMount returns the k8s volume mount.
func (hv HostVolume) VolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      hv.name,
		MountPath: hv.mountPath,
		ReadOnly:  hv.readOnly,
	}
}

// Volume returns the k8s volume.
func (hv HostVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: hv.name,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: hv.hostPath,
				Type: hv.hostPathType,
			},
		},
	}
}

// Name returns the name of the volume
func (hv HostVolume) Name() string {
	return hv.name
}

var _ VolumeLike = HostVolume{}
