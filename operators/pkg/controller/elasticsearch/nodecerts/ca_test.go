// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package nodecerts

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace = "ns1"
	testName      = "mycluster"
)

func Test_certIsValid(t *testing.T) {
	tests := []struct {
		name string
		cert x509.Certificate
		want bool
	}{
		{
			name: "valid cert",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(24 * time.Hour),
			},
			want: true,
		},
		{
			name: "already expired",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(-2 * time.Hour),
			},
			want: false,
		},
		{
			name: "expires soon",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(2 * time.Minute),
			},
			want: false,
		},
		{
			name: "not yet valid",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(10 * time.Minute),
				NotAfter:  time.Now().Add(24 * time.Hour),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := certIsValid(tt.cert); got != tt.want {
				t.Errorf("certIsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_canReuseCa(t *testing.T) {
	tests := []struct {
		name string
		ca   func() certificates.Ca
		want bool
	}{
		{
			name: "valid ca",
			ca: func() certificates.Ca {
				testCa, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{})
				require.NoError(t, err)
				return *testCa
			},
			want: true,
		},
		{
			name: "expired ca",
			ca: func() certificates.Ca {
				testCa, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{})
				require.NoError(t, err)
				testCa.Cert.NotAfter = time.Now().Add(-1 * time.Hour)
				return *testCa
			},
			want: false,
		},
		{
			name: "cert public key & private key misatch",
			ca: func() certificates.Ca {
				testCa, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{})
				require.NoError(t, err)
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				testCa.PrivateKey = privateKey2
				return *testCa
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canReuseCa(tt.ca()); got != tt.want {
				t.Errorf("canReuseCa() = %v, want %v", got, tt.want)
			}
		})
	}
}

func checkCASecrets(
	t *testing.T,
	client k8s.Client,
	cluster v1alpha1.ElasticsearchCluster,
	ca certificates.Ca,
	expectedCa *certificates.Ca,
	notExpectedCa *certificates.Ca,
	expectedExpiration time.Duration,
) {
	// ca cert should be valid
	require.True(t, certIsValid(*ca.Cert))

	// expiration date should be correctly set
	require.True(t, ca.Cert.NotBefore.After(time.Now().Add(-1*time.Hour)))
	require.True(t, ca.Cert.NotAfter.Before(time.Now().Add(1*time.Minute+expectedExpiration)))

	// if an expected Ca was passed, it should match ca
	if expectedCa != nil {
		require.True(t, ca.Cert.Equal(expectedCa.Cert))
		require.Equal(t, ca.PrivateKey.E, expectedCa.PrivateKey.E)
		require.Equal(t, ca.PrivateKey.N, expectedCa.PrivateKey.N)
	}

	// if a not expected Ca was passed, it should not match ca
	if notExpectedCa != nil {
		require.False(t, ca.Cert.Equal(notExpectedCa.Cert))
	}

	// cert and private key should be updated in the apiserver
	certSecret := corev1.Secret{}
	err := client.Get(types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      CACertSecretName(cluster.Name),
	}, &certSecret)
	require.NoError(t, err)
	require.NotEmpty(t, certSecret.Data[certificates.CAFileName])

	privateKeySecret := corev1.Secret{}
	err = client.Get(types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caPrivateKeySecretName(cluster.Name),
	}, &privateKeySecret)
	require.NoError(t, err)
	require.NotEmpty(t, privateKeySecret.Data[CaPrivateKeyFileName])

	// both secrets should be ok to parse as a CA
	parsedCa, ok := caFromSecrets(certSecret, privateKeySecret)
	require.True(t, ok)
	// and return the ca
	require.True(t, ca.Cert.Equal(parsedCa.Cert))
	require.Equal(t, ca.PrivateKey.E, parsedCa.PrivateKey.E)
	require.Equal(t, ca.PrivateKey.N, parsedCa.PrivateKey.N)
}

func Test_renewCA(t *testing.T) {
	cluster := v1alpha1.ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
	testCa, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{})
	require.NoError(t, err)
	privateKeySecret, certSecret := secretsForCa(*testCa, k8s.ExtractNamespacedName(&cluster))

	err = v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	tests := []struct {
		name        string
		client      k8s.Client
		expireIn    time.Duration
		notExpected *certificates.Ca
	}{
		{
			name:     "create new CA",
			client:   k8s.WrapClient(fake.NewFakeClient()),
			expireIn: certificates.DefaultCAValidity,
		},
		{
			name:        "replace existing CA",
			client:      k8s.WrapClient(fake.NewFakeClient(&privateKeySecret, &certSecret)),
			expireIn:    certificates.DefaultCAValidity,
			notExpected: testCa, // existing CA should be replaced
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, err := renewCA(tt.client, cluster, tt.expireIn, scheme.Scheme)
			require.NoError(t, err)
			require.NotNil(t, ca)
			checkCASecrets(t, tt.client, cluster, *ca, nil, tt.notExpected, tt.expireIn)
		})
	}
}

func TestReconcileCAForCluster(t *testing.T) {
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)
	cluster := v1alpha1.ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
	validCa, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{})
	require.NoError(t, err)
	privateKeySecret, certSecret := secretsForCa(*validCa, k8s.ExtractNamespacedName(&cluster))

	soonToExpire := 1 * time.Minute
	soonToExpireCa, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{
		ExpireIn: &soonToExpire,
	})
	require.NoError(t, err)
	soonToExpirePrivateKeySecret, soonToExpireCertSecret := secretsForCa(*soonToExpireCa, k8s.ExtractNamespacedName(&cluster))

	tests := []struct {
		name             string
		cl               k8s.Client
		caCertValidity   time.Duration
		shouldReuseCa    *certificates.Ca // ca that should be reused
		shouldNotReuseCa *certificates.Ca // ca that should not be reused
	}{
		{
			name:           "no existing CA cert nor private key",
			cl:             k8s.WrapClient(fake.NewFakeClient()),
			caCertValidity: certificates.DefaultCAValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing CA cert but no private key",
			cl:             k8s.WrapClient(fake.NewFakeClient(&certSecret)),
			caCertValidity: certificates.DefaultCAValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing private key cert but no cert",
			cl:             k8s.WrapClient(fake.NewFakeClient(&privateKeySecret)),
			caCertValidity: certificates.DefaultCAValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing cert and private key",
			cl:             k8s.WrapClient(fake.NewFakeClient(&privateKeySecret, &certSecret)),
			caCertValidity: certificates.DefaultCAValidity,
			shouldReuseCa:  validCa, // should reuse existing one
		},
		{
			name:             "existing cert is soon to expire",
			cl:               k8s.WrapClient(fake.NewFakeClient(&soonToExpirePrivateKeySecret, &soonToExpireCertSecret)),
			caCertValidity:   certificates.DefaultCAValidity,
			shouldReuseCa:    nil,            // should create a new one
			shouldNotReuseCa: soonToExpireCa, // and not reuse existing one
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, err := ReconcileCAForCluster(tt.cl, cluster, scheme.Scheme, tt.caCertValidity)
			require.NoError(t, err)
			require.NotNil(t, ca)
			checkCASecrets(t, tt.cl, cluster, *ca, tt.shouldReuseCa, tt.shouldNotReuseCa, tt.caCertValidity)
		})
	}
}
