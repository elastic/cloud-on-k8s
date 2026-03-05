// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
)

func Test_shouldIssueNewCertificate(t *testing.T) {
	type args struct {
		secret       corev1.Secret
		pod          *corev1.Pod
		rotateBefore time.Duration
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "missing cert in secret",
			args: args{
				secret:       corev1.Secret{},
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "invalid cert data",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						PodCertFileName(testPod.Name): []byte("invalid"),
					},
				},
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "pod name mismatch",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						PodCertFileName(testPod.Name): rsaCert,
					},
				},
				pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "different"}},
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						PodCertFileName(testPod.Name): rsaCert,
					},
				},
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: false,
		},
		{
			name: "should be rotated soon",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						PodCertFileName(testPod.Name): rsaCert,
					},
				},
				rotateBefore: certificates.DefaultCertValidity, // rotate before the same duration as total validity
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.pod == nil {
				tt.args.pod = &testPod
			}

			if got := shouldIssueNewCertificate(
				context.Background(),
				testES,
				tt.args.secret,
				*tt.args.pod,
				testRSAPrivateKey,
				testRSACA,
				tt.args.rotateBefore,
			); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_shouldIssueNewCertificate_WhenCARotatedWithSamePrivateKey(t *testing.T) {
	// This test verifies that when the CA is renewed using the same private key,
	// certificates signed by the old CA are correctly detected as needing reissuance.
	// Go's x509.Verify() alone would pass because both CAs share the same key pair,
	// but we now also check that the CA certificate in the chain matches the current CA.

	// Create the original CA
	originalCA, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-ca", OrganizationalUnit: []string{"test"}},
		PrivateKey: testRSAPrivateKey,
	})
	require.NoError(t, err)

	// Create a pod certificate signed by the original CA
	podPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)

	podCSRBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, podPrivateKey)
	require.NoError(t, err)
	podCSR, err := x509.ParseCertificateRequest(podCSRBytes)
	require.NoError(t, err)

	podCertTemplate, err := createValidatedCertificateTemplate(testPod, testES, podCSR, certificates.DefaultCertValidity)
	require.NoError(t, err)

	podCertData, err := originalCA.CreateCertificate(*podCertTemplate)
	require.NoError(t, err)

	// The pod's tls.crt contains both the leaf cert AND the CA cert (chain)
	podCertWithChain := certificates.EncodePEMCert(podCertData, originalCA.Cert.Raw)

	// Now simulate CA rotation: create a NEW CA with the SAME private key
	// This is what ECK does when the CA is expiring
	rotatedCA, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-ca", OrganizationalUnit: []string{"test"}},
		PrivateKey: testRSAPrivateKey, // Same private key!
	})
	require.NoError(t, err)

	// Verify the CAs have the same public key but different certificates
	require.Equal(t, originalCA.Cert.PublicKey, rotatedCA.Cert.PublicKey, "CAs should have same public key")
	require.NotEqual(t, originalCA.Cert.Raw, rotatedCA.Cert.Raw, "CA certificates should be different")
	require.NotEqual(t, originalCA.Cert.SerialNumber, rotatedCA.Cert.SerialNumber, "CA serial numbers should be different")

	secret := corev1.Secret{
		Data: map[string][]byte{
			PodCertFileName(testPod.Name): podCertWithChain,
			PodKeyFileName(testPod.Name):  mustEncodePEMPrivateKey(t, podPrivateKey),
		},
	}

	// shouldIssueNewCertificate should detect that the CA in the chain doesn't match
	// the current CA and trigger reissuance
	result := shouldIssueNewCertificate(
		context.Background(),
		testES,
		secret,
		testPod,
		podPrivateKey,
		rotatedCA, // Using the ROTATED CA
		certificates.DefaultRotateBefore,
	)

	assert.True(t, result, "Should issue new certificate when CA has rotated, even if same key")
}

func mustEncodePEMPrivateKey(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	data, err := certificates.EncodePEMPrivateKey(key)
	require.NoError(t, err)
	return data
}

func Test_ensureTransportCertificatesSecretContentsForPod(t *testing.T) {
	tests := []struct {
		name       string
		secret     *corev1.Secret
		pod        *corev1.Pod
		assertions func(t *testing.T, before corev1.Secret, after corev1.Secret)
		wantErr    func(t *testing.T, err error)
	}{
		{
			name: "no private key in the secret",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					PodCertFileName(testPod.Name): rsaCert,
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				t.Helper()
				assert.NotEmpty(t, after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEmpty(t, after.Data[PodCertFileName(testPod.Name)])

				// cert should be re-generated
				assert.NotEqual(t, after.Data[PodCertFileName(testPod.Name)], before.Data[PodCertFileName(testPod.Name)])
			},
		},
		{
			name: "no cert in the secret",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					PodKeyFileName(testPod.Name): testRSAPEMPrivateKey,
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				t.Helper()
				assert.NotEmpty(t, after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEmpty(t, after.Data[PodCertFileName(testPod.Name)])

				// key should be re-used
				assert.Equal(t, before.Data[PodKeyFileName(testPod.Name)], after.Data[PodKeyFileName(testPod.Name)])
			},
		},
		{
			name: "cert does not belong to the key in the secret",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					PodKeyFileName(testPod.Name):  testRSAPEMPrivateKey,
					PodCertFileName(testPod.Name): certificates.EncodePEMCert(testRSACA.Cert.Raw),
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				t.Helper()
				assert.NotEmpty(t, after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEmpty(t, after.Data[PodCertFileName(testPod.Name)])

				// key should be re-used
				assert.Equal(t, before.Data[PodKeyFileName(testPod.Name)], after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEqual(t, after.Data[PodCertFileName(testPod.Name)], before.Data[PodCertFileName(testPod.Name)])
			},
		},
		{
			name: "invalid cert in the secret",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					PodKeyFileName(testPod.Name):  testRSAPEMPrivateKey,
					PodCertFileName(testPod.Name): []byte("invalid"),
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				t.Helper()
				assert.NotEmpty(t, after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEmpty(t, after.Data[PodCertFileName(testPod.Name)])

				// key should be re-used
				assert.Equal(t, before.Data[PodKeyFileName(testPod.Name)], after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEqual(t, after.Data[PodCertFileName(testPod.Name)], before.Data[PodCertFileName(testPod.Name)])
			},
		},
		{
			name: "valid data should not require updating",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					PodKeyFileName(testPod.Name):  testRSAPEMPrivateKey,
					PodCertFileName(testPod.Name): rsaCert,
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				t.Helper()
				assert.Equal(t, before, after)
			},
		},
		{
			name: "ECDSA key should be replaced by a RSA private key",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					PodKeyFileName(testPod.Name):  testECDSAPEMPrivateKey,
					PodCertFileName(testPod.Name): rsaCert,
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				t.Helper()
				assert.NotEmpty(t, after.Data[PodKeyFileName(testPod.Name)])
				assert.NotEmpty(t, after.Data[PodCertFileName(testPod.Name)])

				// both key and cert should be re-generated
				assert.NotEqual(t, after.Data[PodKeyFileName(testPod.Name)], before.Data[PodKeyFileName(testPod.Name)])
				assert.NotEqual(t, after.Data[PodCertFileName(testPod.Name)], before.Data[PodCertFileName(testPod.Name)])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.secret == nil {
				tt.secret = &corev1.Secret{}
			}
			if tt.pod == nil {
				tt.pod = testPod.DeepCopy()
			}

			beforeSecret := tt.secret.DeepCopy()

			err := ensureTransportCertificatesSecretContentsForPod(
				context.Background(),
				testES,
				tt.secret,
				*tt.pod,
				testRSACA,
				certificates.RotationParams{
					Validity:     certificates.DefaultCertValidity,
					RotateBefore: certificates.DefaultRotateBefore,
				},
			)
			if tt.wantErr != nil {
				tt.wantErr(t, err)
				return
			}
			require.NoError(t, err)

			tt.assertions(t, *beforeSecret, *tt.secret)
		})
	}
}
