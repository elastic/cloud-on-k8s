// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var testNamer = name.Namer{
	MaxNameLength:   validation.LabelValueMaxLength,
	MaxSuffixLength: 27, // from a prefix length of 36
	DefaultSuffixes: []string{"test"},
}

var (
	testNamespace = "test-namespace"
	testName      = "test-name"
	testCluster   = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
)

func Test_certIsValid(t *testing.T) {
	tests := []struct {
		name         string
		cert         x509.Certificate
		safetyMargin time.Duration
		want         bool
	}{
		{
			name: "valid cert",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(24 * time.Hour),
			},
			safetyMargin: 1 * time.Hour,
			want:         true,
		},
		{
			name: "already expired",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(-2 * time.Hour),
			},
			safetyMargin: 1 * time.Hour,
			want:         false,
		},
		{
			name: "expires soon",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(30 * time.Minute),
			},
			safetyMargin: 1 * time.Hour,
			want:         false,
		},
		{
			name: "not yet valid",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(10 * time.Minute),
				NotAfter:  time.Now().Add(24 * time.Hour),
			},
			safetyMargin: 1 * time.Hour,
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CertIsValid(context.Background(), tt.cert, tt.safetyMargin); got != tt.want {
				t.Errorf("CertIsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_canReuseCA(t *testing.T) {
	tests := []struct {
		name string
		ca   func() *CA
		want bool
	}{
		{
			name: "valid ca",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				return testCa
			},
			want: true,
		},
		{
			name: "expired ca",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				testCa.Cert.NotAfter = time.Now().Add(-1 * time.Hour)
				return testCa
			},
			want: false,
		},
		{
			name: "cert public key & private key mismatch",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				testCa.PrivateKey = privateKey2
				return testCa
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanReuseCA(context.Background(), tt.ca(), DefaultRotateBefore); got != tt.want {
				t.Errorf("CanReuseCA() = %v, want %v", got, tt.want)
			}
		})
	}
}

func privateKeysEqual(t *testing.T, actual, expected crypto.Signer) {
	t.Helper()
	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Fatalf("unexpected RSA private key, got %T, want %T", actual, expected)
	}
	switch epk := expected.(type) {
	case *rsa.PrivateKey:
		require.True(t, epk.Equal(actual), "private keys should match")
	case *ecdsa.PrivateKey:
		require.True(t, epk.Equal(actual), "private keys should match")
	default:
		t.Fatalf("unexpected RSA private key, got %T, want %T", actual, expected)
	}
}

func checkCASecrets(
	t *testing.T,
	client k8s.Client,
	cluster esv1.Elasticsearch,
	caType CAType,
	ca *CA,
	expectedCa *CA,
	notExpectedCa *CA,
	expectedExpiration time.Duration,
	expectPrivateKey *rsa.PrivateKey,
) {
	t.Helper()
	// ca cert should be valid
	require.True(t, CertIsValid(context.Background(), *ca.Cert, DefaultRotateBefore))

	// expiration date should be correctly set
	require.True(t, ca.Cert.NotBefore.After(time.Now().Add(-1*time.Hour)))
	require.True(t, ca.Cert.NotAfter.Before(time.Now().Add(1*time.Minute+expectedExpiration)))

	// if an expected Ca was passed, it should match ca
	if expectedCa != nil {
		require.True(t, ca.Cert.Equal(expectedCa.Cert))
		privateKeysEqual(t, ca.PrivateKey, expectedCa.PrivateKey)
	}

	// if a not expected Ca was passed, it should not match ca
	if notExpectedCa != nil {
		require.False(t, ca.Cert.Equal(notExpectedCa.Cert))
	}

	if expectPrivateKey != nil {
		privateKeysEqual(t, ca.PrivateKey, expectPrivateKey)
	}

	// cert and private key should be updated in the apiserver
	internalCASecret := corev1.Secret{}
	err := client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      CAInternalSecretName(testNamer, cluster.Name, caType),
	}, &internalCASecret)
	require.NoError(t, err)
	require.NotEmpty(t, internalCASecret.Data[CertFileName])
	require.NotEmpty(t, internalCASecret.Data[KeyFileName])

	// secret should be ok to parse as a CA
	parsedCa := BuildCAFromSecret(context.Background(), internalCASecret)
	require.NotNil(t, parsedCa)
	// and return the ca
	require.True(t, ca.Cert.Equal(parsedCa.Cert))
	privateKeysEqual(t, ca.PrivateKey, parsedCa.PrivateKey)
}

func Test_renewCA(t *testing.T) {
	testCa, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)
	internalCASecret, err := internalSecretForCA(testCa, testNamer, &testCluster, metadata.Metadata{}, TransportCAType)
	require.NoError(t, err)

	tests := []struct {
		name        string
		client      k8s.Client
		expireIn    time.Duration
		notExpected *CA
	}{
		{
			name:     "create new CA",
			client:   k8s.NewFakeClient(),
			expireIn: DefaultCertValidity,
		},
		{
			name:        "replace existing CA",
			client:      k8s.NewFakeClient(&internalCASecret),
			expireIn:    DefaultCertValidity,
			notExpected: testCa, // existing CA should be replaced
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, err := renewCA(context.Background(), tt.client, testNamer, &testCluster, metadata.Metadata{}, tt.expireIn, TransportCAType)
			require.NoError(t, err)
			require.NotNil(t, ca)
			assert.Equal(t, ca.Cert.Issuer.CommonName, testName+"-"+string(TransportCAType))
			checkCASecrets(t, tt.client, testCluster, TransportCAType, ca, nil, tt.notExpected, tt.expireIn, nil)
		})
	}
}

func TestReconcileCAForCluster(t *testing.T) {
	validCa, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)
	internalCASecret, err := internalSecretForCA(validCa, testNamer, &testCluster, metadata.Metadata{}, TransportCAType)
	require.NoError(t, err)

	internalCASecretWithoutPrivateKey := internalCASecret.DeepCopy()
	delete(internalCASecretWithoutPrivateKey.Data, KeyFileName)

	internalCASecretWithoutCACert := internalCASecret.DeepCopy()
	delete(internalCASecretWithoutCACert.Data, CertFileName)

	soonToExpire := 1 * time.Minute
	soonToExpireCa, err := NewSelfSignedCA(CABuilderOptions{
		ExpireIn: &soonToExpire,
	})
	require.NoError(t, err)
	soonToExpireInternalCASecret, err := internalSecretForCA(
		soonToExpireCa, testNamer, &testCluster, metadata.Metadata{}, TransportCAType,
	)
	require.NoError(t, err)
	soonToExpireCAPrivateKey, ok := soonToExpireCa.PrivateKey.(*rsa.PrivateKey)
	require.True(t, ok)

	tests := []struct {
		name               string
		cl                 k8s.Client
		caCertValidity     time.Duration
		shouldReuseCa      *CA             // ca that should be reused
		shouldNotReuseCa   *CA             // ca that should not be reused
		expectedPrivateKey *rsa.PrivateKey // the private key that is expected to be used to create the CA
	}{
		{
			name:           "no existing CA cert nor private key",
			cl:             k8s.NewFakeClient(),
			caCertValidity: DefaultCertValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing CA cert but no private key",
			cl:             k8s.NewFakeClient(internalCASecretWithoutPrivateKey),
			caCertValidity: DefaultCertValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing private key cert but no cert",
			cl:             k8s.NewFakeClient(internalCASecretWithoutCACert),
			caCertValidity: DefaultCertValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing valid internal secret",
			cl:             k8s.NewFakeClient(&internalCASecret),
			caCertValidity: DefaultCertValidity,
			shouldReuseCa:  validCa, // should reuse existing one
		},
		{
			name:               "existing internal cert is soon to expire, and the existing private key will be used to regenerate",
			cl:                 k8s.NewFakeClient(&soonToExpireInternalCASecret),
			caCertValidity:     DefaultCertValidity,
			shouldReuseCa:      nil,                      // should create a new one
			shouldNotReuseCa:   soonToExpireCa,           // and not reuse existing one
			expectedPrivateKey: soonToExpireCAPrivateKey, // the private key that should be used to regenerate a new CA
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, err := ReconcileCAForOwner(
				context.Background(),
				tt.cl, testNamer, &testCluster, metadata.Metadata{}, TransportCAType, RotationParams{
					Validity:     tt.caCertValidity,
					RotateBefore: DefaultRotateBefore,
				},
			)
			require.NoError(t, err)
			require.NotNil(t, ca)
			checkCASecrets(
				t, tt.cl, testCluster, TransportCAType, ca, tt.shouldReuseCa, tt.shouldNotReuseCa, tt.caCertValidity, tt.expectedPrivateKey,
			)
		})
	}
}

func Test_internalSecretForCA(t *testing.T) {
	testCa, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)

	labels := map[string]string{"foo": "bar"}

	internalSecret, err := internalSecretForCA(testCa, testNamer, &testCluster, metadata.Metadata{Labels: labels}, TransportCAType)
	require.NoError(t, err)

	assert.Equal(t, testNamespace, internalSecret.Namespace)
	assert.Equal(t, testName+"-test-transport-ca-internal", internalSecret.Name)
	assert.Len(t, internalSecret.Data, 2)

	assert.NotEmpty(t, internalSecret.Data[CertFileName])
	assert.NotEmpty(t, internalSecret.Data[KeyFileName])

	assert.Equal(t, labels, internalSecret.Labels)
}

func Test_renewCAFromExisting_PreservesSubjectKeyId(t *testing.T) {
	// This test verifies that when a CA is renewed using the same private key,
	// the Subject Key Identifier (SKI) is preserved.

	// Create an initial CA with a specific private key
	initialCa, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, initialCa.Cert.SubjectKeyId, "Initial CA should have a SubjectKeyID")

	// Store it in a secret
	internalCASecret, err := internalSecretForCA(initialCa, testNamer, &testCluster, metadata.Metadata{}, TransportCAType)
	require.NoError(t, err)

	client := k8s.NewFakeClient(&internalCASecret)

	// Simulate CA renewal by calling renewCAFromExisting
	newExpiry := 24 * time.Hour
	renewedCa, err := renewCAFromExisting(
		context.Background(),
		client,
		testNamer,
		&testCluster,
		metadata.Metadata{},
		newExpiry,
		TransportCAType,
		initialCa,
	)
	require.NoError(t, err)
	require.NotNil(t, renewedCa)

	// Verify that the SKI is preserved
	assert.Equal(t, initialCa.Cert.SubjectKeyId, renewedCa.Cert.SubjectKeyId,
		"Subject Key Identifier should be preserved during CA renewal")

	// Verify that other properties are as expected
	assert.NotEqual(t, initialCa.Cert.SerialNumber, renewedCa.Cert.SerialNumber,
		"Serial number should be different for renewed CA")
	assert.Greater(t, initialCa.Cert.NotAfter, renewedCa.Cert.NotAfter,
		"Expiration should be different for renewed CA")

	// Verify private key is the same
	privateKeysEqual(t, renewedCa.PrivateKey, initialCa.PrivateKey)
}

func Test_NewSelfSignedCA_WithSubjectKeyId(t *testing.T) {
	// Test that providing a SubjectKeyID in options results in that SKI being used
	customSKI := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
		0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14}

	ca, err := NewSelfSignedCA(CABuilderOptions{
		SubjectKeyID: customSKI,
	})
	require.NoError(t, err)
	require.NotNil(t, ca)

	assert.Equal(t, customSKI, ca.Cert.SubjectKeyId,
		"CA should use the provided SubjectKeyID")
}

func Test_NewSelfSignedCA_WithoutSubjectKeyId(t *testing.T) {
	// Test that not providing a SubjectKeyID results in Go auto-generating one
	ca, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)
	require.NotNil(t, ca)

	assert.NotEmpty(t, ca.Cert.SubjectKeyId,
		"CA should have an auto-generated SubjectKeyID when none provided")
}

func Test_buildCAFromSecret(t *testing.T) {
	testCa, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)

	internalSecret, err := internalSecretForCA(testCa, testNamer, &testCluster, metadata.Metadata{}, TransportCAType)
	require.NoError(t, err)

	internalSecretMissingCert := internalSecret.DeepCopy()
	delete(internalSecretMissingCert.Data, CertFileName)

	internalSecretMissingPrivateKey := internalSecret.DeepCopy()
	delete(internalSecretMissingPrivateKey.Data, KeyFileName)

	tests := []struct {
		name           string
		internalSecret corev1.Secret
		wantCa         *CA
	}{
		{
			name:           "valid secret",
			internalSecret: internalSecret,
			wantCa:         testCa,
		},
		{
			name:           "empty secret",
			internalSecret: corev1.Secret{},
			wantCa:         nil,
		},
		{
			name:           "secret missing cert",
			internalSecret: *internalSecretMissingCert,
			wantCa:         nil,
		},
		{
			name:           "secret missing private key",
			internalSecret: *internalSecretMissingCert,
			wantCa:         nil,
		},
		{
			name: "invalid cert",
			internalSecret: corev1.Secret{
				Data: map[string][]byte{
					CertFileName: []byte("invalid"),
					KeyFileName:  internalSecret.Data[KeyFileName],
				},
			},
			wantCa: nil,
		},
		{
			name: "invalid private key secret",
			internalSecret: corev1.Secret{
				Data: map[string][]byte{
					CertFileName: internalSecret.Data[CertFileName],
					KeyFileName:  []byte("invalid"),
				},
			},
			wantCa: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := BuildCAFromSecret(context.Background(), tt.internalSecret)
			if tt.wantCa == nil {
				require.Nil(t, ca)
			} else {
				require.NotNil(t, ca)
				assert.True(t, ca.Cert.Equal(tt.wantCa.Cert), "certificates should be equal")
				privateKeysEqual(t, ca.PrivateKey, tt.wantCa.PrivateKey)
			}
		})
	}
}

func TestCertIsSignedByCA(t *testing.T) {
	// Generate test private keys
	caKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	leafKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)

	// Create a CA with a known SKI
	ca1, err := NewSelfSignedCA(CABuilderOptions{
		PrivateKey: caKey,
	})
	require.NoError(t, err)

	// Create a CA with the same SKI (simulating SKI pinning during ECK-managed rotation)
	ca2SameSKI, err := NewSelfSignedCA(CABuilderOptions{
		PrivateKey:   caKey,
		SubjectKeyID: ca1.Cert.SubjectKeyId, // Same SKI
	})
	require.NoError(t, err)

	// Create a CA with a different SKI (simulating custom CA or cross-Go-version scenarios)
	differentSKI := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	ca3DifferentSKI, err := NewSelfSignedCA(CABuilderOptions{
		PrivateKey:   caKey,
		SubjectKeyID: differentSKI, // Different SKI
	})
	require.NoError(t, err)

	// Create a completely different CA (different private key) to test signature verification
	differentCAKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	differentCA, err := NewSelfSignedCA(CABuilderOptions{
		PrivateKey: differentCAKey,
	})
	require.NoError(t, err)

	// Create a leaf certificate signed by ca1 (AKI will match ca1's SKI)
	leafTemplate := ValidatedCertificateTemplate{
		Subject: pkix.Name{
			CommonName: "test-leaf",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		PublicKeyAlgorithm:    x509.RSA,
		SignatureAlgorithm:    x509.SHA256WithRSA,
		SerialNumber:          big.NewInt(1),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  false,
		PublicKey:             leafKey.Public(),
	}

	// Sign leaf with ca1 - the leaf's AKI will be set to ca1's SKI
	leafCertDER, err := ca1.CreateCertificate(leafTemplate)
	require.NoError(t, err)
	leafCert, err := x509.ParseCertificate(leafCertDER)
	require.NoError(t, err)
	require.Equal(t, ca1.Cert.SubjectKeyId, leafCert.AuthorityKeyId, "leaf AKI should match CA SKI")

	// Create a leaf certificate with no AKI (legacy cert simulation)
	leafNoAKITemplate := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "test-leaf-no-aki",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		PublicKeyAlgorithm:    x509.RSA,
		SignatureAlgorithm:    x509.SHA256WithRSA,
		SerialNumber:          big.NewInt(2),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  false,
		// AuthorityKeyId intentionally not set
	}
	leafNoAKIDER, err := x509.CreateCertificate(cryptorand.Reader, &leafNoAKITemplate, &leafNoAKITemplate, leafKey.Public(), leafKey)
	require.NoError(t, err)
	leafNoAKI, err := x509.ParseCertificate(leafNoAKIDER)
	require.NoError(t, err)
	require.Empty(t, leafNoAKI.AuthorityKeyId, "leaf should have no AKI")

	tests := []struct {
		name      string
		leafCert  *x509.Certificate
		currentCA *x509.Certificate
		want      bool
	}{
		{
			name:      "both nil",
			leafCert:  nil,
			currentCA: nil,
			want:      false,
		},
		{
			name:      "leaf nil",
			leafCert:  nil,
			currentCA: ca1.Cert,
			want:      false,
		},
		{
			name:      "currentCA nil",
			leafCert:  leafCert,
			currentCA: nil,
			want:      false,
		},
		{
			name:      "leaf AKI matches CA SKI (same CA)",
			leafCert:  leafCert,
			currentCA: ca1.Cert,
			want:      true,
		},
		{
			name:      "leaf AKI matches CA SKI (rotated CA with same SKI)",
			leafCert:  leafCert,
			currentCA: ca2SameSKI.Cert,
			want:      true, // AKI→SKI match, PKIX validation will pass
		},
		{
			name:      "leaf AKI does NOT match CA SKI (different SKI - byte-swapped)",
			leafCert:  leafCert,
			currentCA: ca3DifferentSKI.Cert,
			want:      false, // AKI→SKI mismatch, PKIX validation would fail
		},
		{
			name:      "leaf without AKI but CA has SKI - triggers reissuance",
			leafCert:  leafNoAKI,
			currentCA: ca1.Cert,
			want:      false, // CA has SKI, so we require leaf to have AKI - triggers reissuance
		},
		{
			name:      "signature verification fails (different CA)",
			leafCert:  leafCert,
			currentCA: differentCA.Cert,
			want:      false, // Signature doesn't match - different private key
		},
		{
			name:     "CA without SKI but leaf has AKI - inconsistent state",
			leafCert: leafCert, // Has AKI pointing to ca1's SKI
			currentCA: &x509.Certificate{
				// Mock a CA without SKI but with same public key for signature verification
				PublicKey:               ca1.Cert.PublicKey,
				SubjectKeyId:            nil, // No SKI
				RawTBSCertificate:       ca1.Cert.RawTBSCertificate,
				Signature:               ca1.Cert.Signature,
				SignatureAlgorithm:      ca1.Cert.SignatureAlgorithm,
				PublicKeyAlgorithm:      ca1.Cert.PublicKeyAlgorithm,
				Raw:                     ca1.Cert.Raw,
				RawSubjectPublicKeyInfo: ca1.Cert.RawSubjectPublicKeyInfo,
			},
			want: false, // Leaf has AKI but CA has no SKI - triggers reissuance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CertIsSignedByCA(tt.leafCert, tt.currentCA)
			assert.Equal(t, tt.want, got)
		})
	}
}
