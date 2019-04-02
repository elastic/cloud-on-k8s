// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"
	"path"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	EnvSourceDir         = "KEYSTORE_SOURCE_DIR"
	EnvKeystoreBinary    = "KEYSTORE_BINARY"
	EnvKeystorePath      = "KEYSTORE_PATH"
	EnvReloadCredentials = "KEYSTORE_RELOAD_CREDENTIALS"
	EnvEsUsername        = "KEYSTORE_ES_USERNAME"
	EnvEsPassword        = "KEYSTORE_ES_PASSWORD"
	EnvEsPasswordFile    = "KEYSTORE_ES_PASSWORD_FILE"
	EnvEsCaCertsPath     = "KEYSTORE_ES_CA_CERTS_PATH"
	EnvEsEndpoint        = "KEYSTORE_ES_ENDPOINT"
	EnvEsVersion         = "KEYSTORE_ES_VERSION"
)

// NewEnvVars returns the environments variables required by the keystore updater.
func NewEnvVars(spec pod.NewPodSpecParams, nodeCertsSecretVolume, reloadCredsUserSecretVolume, keystoreVolume volume.VolumeLike) []corev1.EnvVar {
	esEndpoint := fmt.Sprintf("%s://127.0.0.1:%d", network.ProtocolForLicense(spec.LicenseType), network.HTTPPort)
	return []corev1.EnvVar{
		{Name: EnvSourceDir, Value: keystoreVolume.VolumeMount().MountPath},
		{Name: EnvReloadCredentials, Value: "true"},
		{Name: EnvEsUsername, Value: spec.ReloadCredsUser.Name},
		{Name: EnvEsPasswordFile, Value: path.Join(reloadCredsUserSecretVolume.VolumeMount().MountPath, spec.ReloadCredsUser.Name)},
		{Name: EnvEsCaCertsPath, Value: path.Join(nodeCertsSecretVolume.VolumeMount().MountPath, certificates.CAFileName)},
		{Name: EnvEsEndpoint, Value: esEndpoint},
		{Name: EnvEsVersion, Value: spec.Version},
	}
}
