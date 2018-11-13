package elasticsearch

import (
	"path"

	corev1 "k8s.io/api/core/v1"
)

// Default values for the volume name and paths
const (
	defaultVolumeName        = "volume"
	defaultMountPath         = "/volume"
	defaultSecretMountPath   = "/secrets"
	probeUserSecretMountPath = "/probe-user"
	defaultDataSubDir        = "data"
	defaultLogsSubDir        = "logs"
)

var (
	defaultOptional = false
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

// SecretVolume captures a subset of data of the k8s secrete volume/mount type.
type SecretVolume struct {
	name       string
	mountPath  string
	secretName string
	items      []corev1.KeyToPath
}

// NewSecretVolume creates a new SecretVolume with default mount path.
func NewSecretVolume(secretName string, name string) SecretVolume {
	return NewSecretVolumeWithMountPath(secretName, name, defaultSecretMountPath)
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
