// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

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
	var buf bytes.Buffer
	for _, block := range certBlocks {
		_, _ = buf.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block}))
	}
	return buf.Bytes()
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
		return nil, errors.New("failed to parse PEM block containing private key")
	}

	switch {
	case block.Type == "PRIVATE KEY":
		return parsePKCS8PrivateKey(block.Bytes)
	case block.Type == "RSA PRIVATE KEY" && len(block.Headers) == 0:
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, errors.New("expected PEM block to contain an RSA private key")
	}
}

func parsePKCS8PrivateKey(block []byte) (*rsa.PrivateKey, error) {
	key, err := x509.ParsePKCS8PrivateKey(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse private key")
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected an RSA private key but got %t", key)
	}

	return rsaKey, nil
}
