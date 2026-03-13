// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

const (
	// CertificatesPath is the path to the certificates for the given association.
	CertificatesPath = "/mnt/elastic-internal/%s-association/%s/%s/certs"
)

// CertificatesDir returns the path to the certificates for the given association.
func CertificatesDir(association commonv1.Association) string {
	return fmt.Sprintf(
		CertificatesPath,
		association.AssociationType(),
		association.AssociationRef().Namespace,
		association.AssociationRef().NameOrSecretName(),
	)
}
