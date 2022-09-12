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
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
		t.Errorf("unexpected RSA private key, got %T, want %T", actual, expected)
	}
	switch epk := expected.(type) {
	case *rsa.PrivateKey:
		require.True(t, epk.Equal(expected))
	case *ecdsa.PrivateKey:
		require.True(t, epk.Equal(expected))
	default:
		t.Errorf("unexpected RSA private key, got %T, want %T", actual, expected)
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
	internalCASecret, err := internalSecretForCA(testCa, testNamer, &testCluster, nil, TransportCAType)
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
			ca, err := renewCA(context.Background(), tt.client, testNamer, &testCluster, nil, tt.expireIn, TransportCAType)
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
	internalCASecret, err := internalSecretForCA(validCa, testNamer, &testCluster, nil, TransportCAType)
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
		soonToExpireCa, testNamer, &testCluster, nil, TransportCAType,
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
				tt.cl, testNamer, &testCluster, nil, TransportCAType, RotationParams{
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

	internalSecret, err := internalSecretForCA(testCa, testNamer, &testCluster, labels, TransportCAType)
	require.NoError(t, err)

	assert.Equal(t, testNamespace, internalSecret.Namespace)
	assert.Equal(t, testName+"-test-transport-ca-internal", internalSecret.Name)
	assert.Len(t, internalSecret.Data, 2)

	assert.NotEmpty(t, internalSecret.Data[CertFileName])
	assert.NotEmpty(t, internalSecret.Data[KeyFileName])

	assert.Equal(t, labels, internalSecret.Labels)
}

func Test_buildCAFromSecret(t *testing.T) {
	testCa, err := NewSelfSignedCA(CABuilderOptions{})
	require.NoError(t, err)

	internalSecret, err := internalSecretForCA(testCa, testNamer, &testCluster, nil, TransportCAType)
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
			if !reflect.DeepEqual(ca, tt.wantCa) {
				t.Errorf("CaFromSecrets() got = %v, want %v", ca, tt.wantCa)
			}
		})
	}
}
