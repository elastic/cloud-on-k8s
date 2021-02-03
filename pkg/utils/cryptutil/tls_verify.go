// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cryptutil

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"
)

// VerifyCertificateExceptServerName is a TLS Certificate verification utility method that verifies that the provided
// certificate chain is valid and is signed by one of the root CAs in the provided tls.Config. It is intended to be
// as similar as possible to the default verify (go/src/crypto/tls/handshake_client.go:259), but does not verify
// that the provided certificate matches the ServerName in the tls.Config.
func VerifyCertificateExceptServerName(
	rawCerts [][]byte,
	c *tls.Config,
) ([]*x509.Certificate, [][]*x509.Certificate, error) {
	// this is where we're a bit suboptimal, as we have to re-parse the certificates that have been presented
	// during the handshake.
	// the verification code here is taken from crypto/tls/handshake_client.go:259
	certs := make([]*x509.Certificate, len(rawCerts))
	for i, asn1Data := range rawCerts {
		cert, err := x509.ParseCertificate(asn1Data)
		if err != nil {
			return nil, nil, errors.New("tls: failed to parse certificate from server: " + err.Error())
		}
		certs[i] = cert
	}

	var t time.Time
	if c.Time != nil {
		t = c.Time()
	} else {
		t = time.Now()
	}

	// DNSName omitted in VerifyOptions in order to skip ServerName verification
	opts := x509.VerifyOptions{
		Roots:         c.RootCAs,
		CurrentTime:   t,
		Intermediates: x509.NewCertPool(),
	}

	for i, cert := range certs {
		if i == 0 {
			continue
		}
		opts.Intermediates.AddCert(cert)
	}

	headCert := certs[0]

	// defer to the default verification performed
	chains, err := headCert.Verify(opts)
	return certs, chains, err
}
