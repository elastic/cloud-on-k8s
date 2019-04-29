// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"

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

type NewEnvVarsParams struct {
	SourceDir          string
	ESUsername         string
	ESPasswordFilepath string
	ESVersion          string
	ESCaCertPath       string
}

// NewEnvVars returns the environments variables required by the keystore updater.
func NewEnvVars(params NewEnvVarsParams) []corev1.EnvVar {
	esEndpoint := fmt.Sprintf("https://127.0.0.1:%d", network.HTTPPort)
	return []corev1.EnvVar{
		{Name: EnvSourceDir, Value: params.SourceDir},
		{Name: EnvReloadCredentials, Value: "true"},
		{Name: EnvEsUsername, Value: params.ESUsername},
		{Name: EnvEsPasswordFile, Value: params.ESPasswordFilepath},
		{Name: EnvEsCaCertsPath, Value: params.ESCaCertPath},
		{Name: EnvEsEndpoint, Value: esEndpoint},
		{Name: EnvEsVersion, Value: params.ESVersion},
	}
}
