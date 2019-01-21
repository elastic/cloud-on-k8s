package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// Default values for the volume name and paths
const (
	// DefaultSecretMountPath where secrets are mounted if not specified otherwise.
	DefaultSecretMountPath                = "/secrets"
	ProbeUserSecretMountPath              = "/probe-user"
	ProbeUserVolumeName                   = "probe-user"
	NodeCertificatesSecretVolumeName      = "node-certificates"
	NodeCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/node-certs"
)

var (
	defaultOptional = false
)

type VolumeLike interface {
	Name() string
	Volume() corev1.Volume
	VolumeMount() corev1.VolumeMount
}
