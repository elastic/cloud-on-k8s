// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/mnt/elastic-internal/http-certs" // nolint
)

// HTTPCertSecretVolume returns a SecretVolume to hold the HTTP certs for the given resource.
func HTTPCertSecretVolume(namer name.Namer, name string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		certificates.HTTPCertsInternalSecretName(namer, name),
		HTTPCertificatesSecretVolumeName,
		HTTPCertificatesSecretVolumeMountPath,
	)
}
