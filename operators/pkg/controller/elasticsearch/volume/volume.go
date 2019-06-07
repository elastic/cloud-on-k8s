// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// Default values for the volume name and paths
const (
	ProbeUserSecretMountPath = "/mnt/elastic-internal/probe-user"
	ProbeUserVolumeName      = "elastic-internal-probe-user"

	KeystoreUserSecretMountPath = "/mnt/elastic-internal/keystore-user"
	KeystoreUserVolumeName      = "elsatic-internal-keystore-user"

	TransportCertificatesSecretVolumeName      = "elastic-internal-transport-certificates"
	TransportCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/transport-certs"

	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/http-certs"

	SecureSettingsVolumeName      = "elastic-internal-secure-settings"
	SecureSettingsVolumeMountPath = "/mnt/elastic-internal/secure-settings"

	XPackFileRealmVolumeName      = "elastic-internal-xpack-file-realm"
	XPackFileRealmVolumeMountPath = "/mnt/elastic-internal/xpack-file-realm"

	UnicastHostsVolumeName      = "elastic-internal-unicast-hosts"
	UnicastHostsVolumeMountPath = "/mnt/elastic-internal/unicast-hosts"
	UnicastHostsFile            = "unicast_hosts.txt"

	ProcessManagerEmptyDirMountPath = "/mnt/elastic-internal/process-manager"
)

var (
	defaultOptional = false
)

type VolumeLike interface {
	Name() string
	Volume() corev1.Volume
	VolumeMount() corev1.VolumeMount
}
