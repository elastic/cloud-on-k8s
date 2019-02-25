// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"reflect"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates/certutil"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apiV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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
	// testCa is a self-signed CA intended for testing
	testCa *Ca
	// testRSAPrivateKey is a preconfigured RSA private key intended for testing.
	testRSAPrivateKey *rsa.PrivateKey
)

func init() {
	logf.SetLogger(logf.ZapLogger(false))
	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCa, err = NewSelfSignedCaUsingKey("test", testRSAPrivateKey); err != nil {
		panic("Failed to create new self signed CA: " + err.Error())
	}
}

func TestCa_CreateCertificateForValidatedCertificateTemplate(t *testing.T) {
	// create a certificate template for the csr
	cn := "test-cn"
	certificateTemplate := ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotAfter: time.Now().Add(365 * 24 * time.Hour),

		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          &testRSAPrivateKey.PublicKey,
	})

	bytes, err := testCa.CreateCertificate(certificateTemplate)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(bytes)
	require.NoError(t, err)

	assert.Equal(t, cert.Subject.CommonName, cn)

	// the issued certificate should pass verification
	pool := x509.NewCertPool()
	pool.AddCert(testCa.Cert)
	_, err = cert.Verify(x509.VerifyOptions{
		DNSName: cn,
		Roots:   pool,
	})
	assert.NoError(t, err)
}

func TestReconcilePublicCertsSecret(t *testing.T) {
	fooCa, _ := NewSelfSignedCa("foo")
	nsn := types.NamespacedName{
		Namespace: "ns1",
		Name:      "a-secret-to-update",
	}
	fakeOwner := &corev1.Secret{ObjectMeta: k8s.ToObjectMeta(nsn)}

	// Create an outdated secret
	c := k8s.WrapClient(fake.NewFakeClient(
		&corev1.Secret{
			ObjectMeta: apiV1.ObjectMeta{
				Name:      nsn.Name,
				Namespace: nsn.Namespace,
			},
			Data: map[string][]byte{CAFileName: []byte("awronginitialsupersecret1")},
		}))

	// Reconciliation must update it
	err := fooCa.ReconcilePublicCertsSecret(c, nsn, fakeOwner, scheme.Scheme)
	assert.NoError(t, err, "Ooops")

	// Check if the secret has been updated
	updated := &corev1.Secret{}
	c.Get(nsn, updated)

	expectedCaKeyBytes := certutil.EncodePEMCert(fooCa.Cert.Raw)
	assert.True(t, reflect.DeepEqual(expectedCaKeyBytes, updated.Data[CAFileName]))
}
