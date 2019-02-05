package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// EmptyDirVolume used to store ES data on the node main disk
// Its lifecycle is bound to the pod lifecycle on the node.
type EmptyDirVolume struct {
	name      string
	mountPath string
}

// NewEmptyDirVolume creates an EmptyDirVolume with default values
func NewEmptyDirVolume(name, mountPath string) EmptyDirVolume {
	return EmptyDirVolume{
		name:      name,
		mountPath: mountPath,
	}
}

// Volume returns the associated k8s volume
func (v EmptyDirVolume) Volume() corev1.Volume {
	return corev1.Volume{
		Name: v.name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// VolumeMount returns the associated k8s volume mount
func (v EmptyDirVolume) VolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		MountPath: v.mountPath,
		Name:      v.name,
	}
}

// Name returns the name of the volume
func (v EmptyDirVolume) Name() string {
	return v.name
}

var _ VolumeLike = EmptyDirVolume{}
