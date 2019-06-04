// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

const (
	// CAFileName is used for the CA Certificates inside a secret
	CAFileName = "ca.crt"

	// CertFileName is used for Certificates inside a secret
	CertFileName = "tls.crt"

	// KeyFileName is used for Private Keys inside a secret
	KeyFileName = "tls.key"

	// CSRFileName is used for the CSR inside a secret
	CSRFileName = "tls.csr"
)
