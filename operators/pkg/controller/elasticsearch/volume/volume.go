// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	corev1 "k8s.io/api/core/v1"
)

// Default values for the volume name and paths
const (
	// DefaultSecretMountPath where secrets are mounted if not specified otherwise.
	DefaultSecretMountPath = "/mnt/elastic/secrets"

	ProbeUserSecretMountPath = "/mnt/elastic/probe-user"
	ProbeUserVolumeName      = "probe-user"

	ReloadCredsUserSecretMountPath = "/mnt/elastic/reload-creds-user"
	ReloadCredsUserVolumeName      = "reload-creds-user"

	NodeCertificatesSecretVolumeName      = "node-certificates"
	NodeCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/node-certs"

	SecureSettingsVolumeName      = "secure-settings"
	SecureSettingsVolumeMountPath = "/mnt/elastic/secure-settings"

	ExtraFilesSecretVolumeMountPath = "/usr/share/elasticsearch/config/extrafiles"

	ProcessManagerEmptyDirMountPath = "/mnt/elastic/process-manager"
)

var (
	defaultOptional = false
)

type VolumeLike interface {
	Name() string
	Volume() corev1.Volume
	VolumeMount() corev1.VolumeMount
}
