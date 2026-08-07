// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helper

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// GenerateSelfSignedClientCert generates a self-signed client certificate using ECDSA
// and returns PEM-encoded cert and key.
func GenerateSelfSignedClientCert(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certPEM = generateSelfSignedCert(t, cn, x509.ECDSAWithSHA256, privateKey, x509.ExtKeyUsageClientAuth)
	return certPEM, keyPEM
}

// GenerateSelfSignedClientCertPKCS8 generates a self-signed client certificate using RSA with a PKCS#8-encoded
// private key. Logstash (Java/JRuby) requires PKCS#8 format for client certificate keys.
func GenerateSelfSignedClientCertPKCS8(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)

	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	certPEM = generateSelfSignedCert(t, cn, x509.SHA256WithRSA, privateKey, x509.ExtKeyUsageClientAuth)
	return certPEM, keyPEM
}

// PKCS8KeyEndsWithWhitespaceByte reports whether the last byte of the DER payload inside keyPEM is an ASCII
// whitespace character.
func PKCS8KeyEndsWithWhitespaceByte(keyPEM []byte) (bool, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || len(block.Bytes) == 0 {
		return false, errors.New("empty decoded block")
	}
	last := block.Bytes[len(block.Bytes)-1]
	return last == 0x00 || last == '\t' || last == '\n' || last == '\v' || last == '\f' || last == '\r' || last == ' ', nil
}

// GenerateSelfSignedServerCertPKCS8 generates a self-signed server certificate using ECDSA
// and returns PEM-encoded cert and key. The key is PKCS#8-encoded ("PRIVATE KEY" PEM type)
// as required by the Logstash Beats input.
func GenerateSelfSignedServerCertPKCS8(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)

	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	certPEM = generateSelfSignedCert(t, cn, x509.ECDSAWithSHA256, privateKey, x509.ExtKeyUsageServerAuth)
	return certPEM, keyPEM
}

// generateSelfSignedCert creates a self-signed certificate using the given key, signature algorithm, and extended key usages.
func generateSelfSignedCert(t *testing.T, cn string, sigAlg x509.SignatureAlgorithm, key crypto.Signer, extKeyUsage ...x509.ExtKeyUsage) []byte {
	t.Helper()

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
		ExtKeyUsage:        extKeyUsage,
		SignatureAlgorithm: sigAlg,
	}

	certDER, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, key.Public(), key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}
