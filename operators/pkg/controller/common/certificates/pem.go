// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/pkg/errors"
)

// ParsePEMCerts returns a list of certificates from the given PEM certs data
// Based on the code of x509.CertPool.AppendCertsFromPEM (https://golang.org/src/crypto/x509/cert_pool.go)
// We don't rely on x509.CertPool.AppendCertsFromPEM directly here since it returns an interface from which
// we cannot extract the actual certificates if we need to compare them.
func ParsePEMCerts(pemData []byte) ([]*x509.Certificate, error) {
	certs := []*x509.Certificate{}
	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}

		certs = append(certs, cert)
	}
	return certs, nil
}

// EncodePEMCert encodes the given certificate blocks as a PEM certificate
func EncodePEMCert(certBlocks ...[]byte) []byte {
	var result []byte
	for _, block := range certBlocks {
		result = append(result, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block})...)
	}
	return result
}

// EncodePEMPrivateKey encodes the given private key in the PEM format
func EncodePEMPrivateKey(privateKey rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(&privateKey),
	})
}

// ParsePEMPrivateKey parses the given private key in the PEM format
func ParsePEMPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("can't decode pem block")
	}
	if block.Type != "RSA PRIVATE KEY" || len(block.Headers) != 0 {
		return nil, errors.New("pem block is not an RSA private key")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
