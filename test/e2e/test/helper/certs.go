// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helper

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// GenerateSelfSignedClientCert generates a self-signed client certificate and returns PEM-encoded cert and key.
func GenerateSelfSignedClientCert(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)

	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         cn,
			OrganizationalUnit: []string{"eck-e2e-test"},
		},
		NotBefore:          time.Now().Add(-10 * time.Minute),
		NotAfter:           time.Now().Add(24 * time.Hour),
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	certDER, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, privateKey.Public(), privateKey)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}
