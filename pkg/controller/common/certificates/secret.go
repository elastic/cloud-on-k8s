// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	certsPublicSecretName                 = "certs-public"
	certsInternalSecretName               = "certs-internal"
	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/mnt/elastic-internal/http-certs" // nolint
)

func InternalCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http", certsInternalSecretName)
}

func PublicCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http", certsPublicSecretName)
}

// HTTPCertSecretVolume returns a SecretVolume to hold the HTTP certs for the given resource.
func HTTPCertSecretVolume(namer name.Namer, name string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		InternalCertsSecretName(namer, name),
		HTTPCertificatesSecretVolumeName,
		HTTPCertificatesSecretVolumeMountPath,
	)
}
