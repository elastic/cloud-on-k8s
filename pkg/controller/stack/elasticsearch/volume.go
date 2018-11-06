package elasticsearch

import (
	"path"

	corev1 "k8s.io/api/core/v1"
)

// Default values for the volume name and paths
const (
	defaultVolumeName = "volume"
	defaultMountPath  = "/volume"
	defaultDataSubDir = "data"
	defaultLogsSubDir = "logs"
)

// EmptyDirVolume used to store ES data on the node main disk
// Its lifecycle is bound to the pod lifecycle on the node.
type EmptyDirVolume struct {
	name       string
	mountPath  string
	dataSubDir string
	logsSubDir string
}

// NewDefaultEmptyDirVolume creates an EmptyDirVolume with default values
func NewDefaultEmptyDirVolume() EmptyDirVolume {
	return EmptyDirVolume{
		name:       defaultVolumeName,
		mountPath:  defaultMountPath,
		dataSubDir: defaultDataSubDir,
		logsSubDir: defaultLogsSubDir,
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

// DataPath returns the absolute path to the directory storing ES data
func (v EmptyDirVolume) DataPath() string {
	return path.Join(v.mountPath, v.dataSubDir)
}

// LogsPath returns the absolute path to the directory storing ES logs
func (v EmptyDirVolume) LogsPath() string {
	return path.Join(v.mountPath, v.logsSubDir)
}
