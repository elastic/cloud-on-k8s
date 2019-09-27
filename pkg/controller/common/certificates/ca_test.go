// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package certificates

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPemPrivateKey contains a private key intended for testing
const testPemPrivateKey = `
-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQCxoeCUW5KJxNPxMp+KmCxKLc1Zv9Ny+4CFqcUXVUYH69L3mQ7v
IWrJ9GBfcaA7BPQqUlWxWM+OCEQZH1EZNIuqRMNQVuIGCbz5UQ8w6tS0gcgdeGX7
J7jgCQ4RK3F/PuCM38QBLaHx988qG8NMc6VKErBjctCXFHQt14lerd5KpQIDAQAB
AoGAYrf6Hbk+mT5AI33k2Jt1kcweodBP7UkExkPxeuQzRVe0KVJw0EkcFhywKpr1
V5eLMrILWcJnpyHE5slWwtFHBG6a5fLaNtsBBtcAIfqTQ0Vfj5c6SzVaJv0Z5rOd
7gQF6isy3t3w9IF3We9wXQKzT6q5ypPGdm6fciKQ8RnzREkCQQDZwppKATqQ41/R
vhSj90fFifrGE6aVKC1hgSpxGQa4oIdsYYHwMzyhBmWW9Xv/R+fPyr8ZwPxp2c12
33QwOLPLAkEA0NNUb+z4ebVVHyvSwF5jhfJxigim+s49KuzJ1+A2RaSApGyBZiwS
rWvWkB471POAKUYt5ykIWVZ83zcceQiNTwJBAMJUFQZX5GDqWFc/zwGoKkeR49Yi
MTXIvf7Wmv6E++eFcnT461FlGAUHRV+bQQXGsItR/opIG7mGogIkVXa3E1MCQARX
AAA7eoZ9AEHflUeuLn9QJI/r0hyQQLEtrpwv6rDT1GCWaLII5HJ6NUFVf4TTcqxo
6vdM4QGKTJoO+SaCyP0CQFdpcxSAuzpFcKv0IlJ8XzS/cy+mweCMwyJ1PFEc4FX6
wg/HcAJWY60xZTJDFN+Qfx8ZQvBEin6c2/h+zZi5IVY=
-----END RSA PRIVATE KEY-----
`

var (
	// testCA is a self-signed CA intended for testing
	testCA *CA
	// testRSAPrivateKey is a preconfigured RSA private key intended for testing.
	testRSAPrivateKey *rsa.PrivateKey
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
