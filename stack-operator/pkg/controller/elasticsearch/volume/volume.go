package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// Default values for the volume name and paths
const (
	// DefaultSecretMountPath where secrets are mounted if not specified otherwise.
	DefaultSecretMountPath   = "/secrets"
	ProbeUserSecretMountPath = "/probe-user"
	// KeystoreSecretMountPath Mountpath for keystore secrets in init container.
	KeystoreSecretMountPath = "/keystore-secrets"

	NodeCertificatesSecretVolumeName      = "node-certificates"
	NodeCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/node-certs"
)

var (
	defaultOptional = false
)

type VolumeLike interface {
	Volume() corev1.Volume
	VolumeMount() corev1.VolumeMount
}

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

var _ VolumeLike = EmptyDirVolume{}

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

var _ VolumeLike = SecretVolume{}

// NewConfigMapVolume creates a new ConfigMapVolume struct
func NewConfigMapVolume(name, mountPath string) ConfigMapVolume {
	return ConfigMapVolume{
		name:      name,
		mountPath: mountPath,
	}
}

// ConfigMapVolume
type ConfigMapVolume struct {
	name      string
	mountPath string
	items     []corev1.KeyToPath
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
					Name: cm.name,
				},
				Items:    cm.items,
				Optional: &defaultOptional,
			},
		},
	}
}

var _ VolumeLike = ConfigMapVolume{}
