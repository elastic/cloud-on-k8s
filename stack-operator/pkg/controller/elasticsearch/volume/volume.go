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
