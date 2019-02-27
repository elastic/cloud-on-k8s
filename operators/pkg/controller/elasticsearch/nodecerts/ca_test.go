// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestCreateCaSecret(t *testing.T) {
	secret, ca, err := CreateCaSecret(types.NamespacedName{
		Namespace: "namespace",
		Name:      "name",
	}, certificates.DefaultCAValidity)
	require.NoError(t, err)
	require.NotNil(t, ca.Cert)
	require.Equal(t, secret.Name, "name")
	require.Equal(t, secret.Namespace, "namespace")
	require.Equal(t, ca.Cert.Subject.CommonName, CACommonName)
	require.NotEmpty(t, secret.Data[certificates.CAFileName])
	require.NotEmpty(t, secret.Data[CaPrivateKeyFileName])
}

func TestCaFromSecret(t *testing.T) {
	objMeta := metav1.ObjectMeta{
		Namespace: "namespace",
		Name:      "name",
	}
	tests := []struct {
		name        string
		secret      corev1.Secret
		canBeParsed bool
	}{
		{
			name: "valid secret",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CAFileName: pemCert,
					CaPrivateKeyFileName:    []byte(testPemPrivateKey),
				},
			},
			canBeParsed: true,
		},
		{
			name: "no data",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
			},
			canBeParsed: false,
		},
		{
			name: "no cert",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CaPrivateKeyFileName: []byte(testPemPrivateKey),
				},
			},
			canBeParsed: false,
		},
		{
			name: "no private key",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CAFileName: pemCert,
				},
			},
			canBeParsed: false,
		},
		{
			name: "invalid cert",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CAFileName: []byte("invalid"),
					CaPrivateKeyFileName:    []byte(testPemPrivateKey),
				},
			},
			canBeParsed: false,
		},
		{
			name: "invalid private key",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CAFileName: pemCert,
					CaPrivateKeyFileName:    []byte("invalid"),
				},
			},
			canBeParsed: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, canBeParsed := CaFromSecret(tt.secret)
			require.Equal(t, tt.canBeParsed, canBeParsed)
			if tt.canBeParsed {
				require.NotNil(t, ca)
			}
		})
	}
}

func Test_shouldUpdateCACert(t *testing.T) {
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
			want: false,
		},
		{
			name: "already expired",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(-2 * time.Hour),
			},
			want: true,
		},
		{
			name: "expires soon",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(-1 * time.Minute),
				NotAfter:  time.Now().Add(2 * time.Minute),
			},
			want: true,
		},
		{
			name: "not yet valid",
			cert: x509.Certificate{
				NotBefore: time.Now().Add(10 * time.Minute),
				NotAfter:  time.Now().Add(24 * time.Hour),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUpdateCACert(tt.cert); got != tt.want {
				t.Errorf("shouldUpdateCACert() = %v, want %v", got, tt.want)
			}
		})
	}
}
