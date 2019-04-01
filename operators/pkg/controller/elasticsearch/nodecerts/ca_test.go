// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package nodecerts

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"reflect"
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
			if got := certIsValid(tt.cert, tt.safetyMargin); got != tt.want {
				t.Errorf("certIsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_canReuseCA(t *testing.T) {
	tests := []struct {
		name string
		ca   func() certificates.CA
		want bool
	}{
		{
			name: "valid ca",
			ca: func() certificates.CA {
				testCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
				require.NoError(t, err)
				return *testCa
			},
			want: true,
		},
		{
			name: "expired ca",
			ca: func() certificates.CA {
				testCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
				require.NoError(t, err)
				testCa.Cert.NotAfter = time.Now().Add(-1 * time.Hour)
				return *testCa
			},
			want: false,
		},
		{
			name: "cert public key & private key misatch",
			ca: func() certificates.CA {
				testCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
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
			if got := canReuseCA(tt.ca(), certificates.DefaultRotateBefore); got != tt.want {
				t.Errorf("canReuseCA() = %v, want %v", got, tt.want)
			}
		})
	}
}

func checkCASecrets(
	t *testing.T,
	client k8s.Client,
	cluster v1alpha1.Elasticsearch,
	ca certificates.CA,
	expectedCa *certificates.CA,
	notExpectedCa *certificates.CA,
	expectedExpiration time.Duration,
) {
	// ca cert should be valid
	require.True(t, certIsValid(*ca.Cert, certificates.DefaultRotateBefore))

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
	require.NotEmpty(t, privateKeySecret.Data[CAPrivateKeyFileName])

	// both secrets should be ok to parse as a CA
	parsedCa, ok := caFromSecrets(certSecret, privateKeySecret)
	require.True(t, ok)
	// and return the ca
	require.True(t, ca.Cert.Equal(parsedCa.Cert))
	require.Equal(t, ca.PrivateKey.E, parsedCa.PrivateKey.E)
	require.Equal(t, ca.PrivateKey.N, parsedCa.PrivateKey.N)
}

func Test_renewCA(t *testing.T) {
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
	testCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)
	privateKeySecret, certSecret := secretsForCA(*testCa, k8s.ExtractNamespacedName(&cluster))

	err = v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	tests := []struct {
		name        string
		client      k8s.Client
		expireIn    time.Duration
		notExpected *certificates.CA
	}{
		{
			name:     "create new CA",
			client:   k8s.WrapClient(fake.NewFakeClient()),
			expireIn: certificates.DefaultCertValidity,
		},
		{
			name:        "replace existing CA",
			client:      k8s.WrapClient(fake.NewFakeClient(&privateKeySecret, &certSecret)),
			expireIn:    certificates.DefaultCertValidity,
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
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
	validCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)
	privateKeySecret, certSecret := secretsForCA(*validCa, k8s.ExtractNamespacedName(&cluster))

	soonToExpire := 1 * time.Minute
	soonToExpireCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		ExpireIn: &soonToExpire,
	})
	require.NoError(t, err)
	soonToExpirePrivateKeySecret, soonToExpireCertSecret := secretsForCA(*soonToExpireCa, k8s.ExtractNamespacedName(&cluster))

	tests := []struct {
		name             string
		cl               k8s.Client
		caCertValidity   time.Duration
		shouldReuseCa    *certificates.CA // ca that should be reused
		shouldNotReuseCa *certificates.CA // ca that should not be reused
	}{
		{
			name:           "no existing CA cert nor private key",
			cl:             k8s.WrapClient(fake.NewFakeClient()),
			caCertValidity: certificates.DefaultCertValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing CA cert but no private key",
			cl:             k8s.WrapClient(fake.NewFakeClient(&certSecret)),
			caCertValidity: certificates.DefaultCertValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing private key cert but no cert",
			cl:             k8s.WrapClient(fake.NewFakeClient(&privateKeySecret)),
			caCertValidity: certificates.DefaultCertValidity,
			shouldReuseCa:  nil, // should create a new one
		},
		{
			name:           "existing cert and private key",
			cl:             k8s.WrapClient(fake.NewFakeClient(&privateKeySecret, &certSecret)),
			caCertValidity: certificates.DefaultCertValidity,
			shouldReuseCa:  validCa, // should reuse existing one
		},
		{
			name:             "existing cert is soon to expire",
			cl:               k8s.WrapClient(fake.NewFakeClient(&soonToExpirePrivateKeySecret, &soonToExpireCertSecret)),
			caCertValidity:   certificates.DefaultCertValidity,
			shouldReuseCa:    nil,            // should create a new one
			shouldNotReuseCa: soonToExpireCa, // and not reuse existing one
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, err := ReconcileCAForCluster(tt.cl, cluster, scheme.Scheme, tt.caCertValidity, certificates.DefaultRotateBefore)
			require.NoError(t, err)
			require.NotNil(t, ca)
			checkCASecrets(t, tt.cl, cluster, *ca, tt.shouldReuseCa, tt.shouldNotReuseCa, tt.caCertValidity)
		})
	}
}

func Test_getCA(t *testing.T) {
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
	validCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)
	_, certSecret := secretsForCA(*validCa, k8s.ExtractNamespacedName(&cluster))
	type args struct {
		c  k8s.Client
		es types.NamespacedName
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "CA cert does not exist",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(&certSecret)),
				es: types.NamespacedName{
					Namespace: "default",
					Name:      "foo",
				},
			},
			want: nil,
		}, {
			name: "CA cert does exist",
			args: args{
				c:  k8s.WrapClient(fake.NewFakeClient(&certSecret)),
				es: k8s.ExtractNamespacedName(&cluster),
			},
			want: certificates.EncodePEMCert(validCa.Cert.Raw),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCA(tt.args.c, tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCA() = %v, want %v", got, tt.want)
			}
		})
	}
}
