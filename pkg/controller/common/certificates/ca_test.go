// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package certificates

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCA, err = NewSelfSignedCA(CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test"},
		PrivateKey: testRSAPrivateKey,
	}); err != nil {
		panic("Failed to create new self signed CA: " + err.Error())
	}
}

func TestCA_CreateCertificate(t *testing.T) {
	// create a validated certificate template for the csr
	cn := "test-cn"
	certificateTemplate := ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotAfter: time.Now().Add(365 * 24 * time.Hour),

		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          &testRSAPrivateKey.PublicKey,
	})

	bytes, err := testCA.CreateCertificate(certificateTemplate)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(bytes)
	require.NoError(t, err)

	assert.Equal(t, cert.Subject.CommonName, cn)

	// the issued certificate should pass verification
	pool := x509.NewCertPool()
	pool.AddCert(testCA.Cert)
	_, err = cert.Verify(x509.VerifyOptions{
		DNSName: cn,
		Roots:   pool,
	})
	assert.NoError(t, err)
}

func TestNewSelfSignedCA(t *testing.T) {
	// with no options, should not fail
	ca, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)
	require.NotNil(t, ca)

	// with options, should use them
	expireIn := 1 * time.Hour
	opts := CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-common-name"},
		PrivateKey: testRSAPrivateKey,
		ExpireIn:   &expireIn,
	}

	ca, err = NewSelfSignedCA(opts)
	require.NoError(t, err)
	require.NotNil(t, ca)
	require.Equal(t, ca.Cert.Subject.CommonName, opts.Subject.CommonName)
	require.Equal(t, testRSAPrivateKey, ca.PrivateKey)
	require.True(t, ca.Cert.NotBefore.Before(time.Now().Add(2*time.Hour)))
}
