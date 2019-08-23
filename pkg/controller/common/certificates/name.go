// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

const (
	certsPublicSecretName   = "certs-public"
	certsInternalSecretName = "certs-internal"
)

func PublicSecretName(namer name.Namer, ownerName string, caType CAType) string {
	return namer.Suffix(ownerName, string(caType), certsPublicSecretName)
}

func HTTPCertsInternalSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, string(HTTPCAType), certsInternalSecretName)
}
