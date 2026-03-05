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
	// Helper to create a pod certificate signed by a CA
	createPodCertWithCA := func(t *testing.T, ca *certificates.CA) (secret corev1.Secret, privateKey *rsa.PrivateKey) {
		t.Helper()
		podPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		require.NoError(t, err)

		podCSRBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, podPrivateKey)
		require.NoError(t, err)
		podCSR, err := x509.ParseCertificateRequest(podCSRBytes)
		require.NoError(t, err)

		podCertTemplate, err := createValidatedCertificateTemplate(testPod, testES, podCSR, certificates.DefaultCertValidity)
		require.NoError(t, err)

		podCertData, err := ca.CreateCertificate(*podCertTemplate)
		require.NoError(t, err)

		podCertWithChain := certificates.EncodePEMCert(podCertData, ca.Cert.Raw)
		pemPrivateKey, err := certificates.EncodePEMPrivateKey(podPrivateKey)
		require.NoError(t, err)

		return corev1.Secret{
			Data: map[string][]byte{
				PodCertFileName(testPod.Name): podCertWithChain,
				PodKeyFileName(testPod.Name):  pemPrivateKey,
			},
		}, podPrivateKey
	}

	// Create the original CA for CA rotation tests
	originalCA, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-ca", OrganizationalUnit: []string{"test"}},
		PrivateKey: testRSAPrivateKey,
	})
	require.NoError(t, err)

	// Create rotated CA with DIFFERENT SKI (simulates custom CA or cross-Go-version scenarios)
	differentSKI := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	rotatedCADifferentSKI, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:      pkix.Name{CommonName: "test-ca", OrganizationalUnit: []string{"test"}},
		PrivateKey:   testRSAPrivateKey,
		SubjectKeyID: differentSKI,
	})
	require.NoError(t, err)

	// Create rotated CA with SAME SKI (simulates ECK-managed CA rotation with SKI pinning)
	rotatedCASameSKI, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:      pkix.Name{CommonName: "test-ca", OrganizationalUnit: []string{"test"}},
		PrivateKey:   testRSAPrivateKey,
		SubjectKeyID: originalCA.Cert.SubjectKeyId,
	})
	require.NoError(t, err)

	// Verify CA setup for rotation tests
	require.NotEqual(t, originalCA.Cert.SubjectKeyId, rotatedCADifferentSKI.Cert.SubjectKeyId, "CAs should have different SKIs")
	require.Equal(t, originalCA.Cert.SubjectKeyId, rotatedCASameSKI.Cert.SubjectKeyId, "CAs should have same SKI")

	// Create pod certificate signed by originalCA for CA rotation tests
	secretWithOriginalCA, podPrivateKeyForRotationTests := createPodCertWithCA(t, originalCA)

	type args struct {
		secret       corev1.Secret
		pod          *corev1.Pod
		privateKey   *rsa.PrivateKey  // optional, defaults to testRSAPrivateKey
		ca           *certificates.CA // optional, defaults to testRSACA
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
		{
			name: "CA rotated with different SKI - should reissue",
			args: args{
				secret:       secretWithOriginalCA,
				privateKey:   podPrivateKeyForRotationTests,
				ca:           rotatedCADifferentSKI, // rotated CA has different SKI
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true, // different SKI means we need to reissue
		},
		{
			name: "CA rotated with same SKI - should NOT reissue",
			args: args{
				secret:       secretWithOriginalCA,
				privateKey:   podPrivateKeyForRotationTests,
				ca:           rotatedCASameSKI, // rotated CA has same SKI (pinned)
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: false, // same SKI means the chain is still valid, no reissuance needed
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := tt.args.pod
			if pod == nil {
				pod = &testPod
			}
			privateKey := tt.args.privateKey
			if privateKey == nil {
				privateKey = testRSAPrivateKey
			}
			ca := tt.args.ca
			if ca == nil {
				ca = testRSACA
			}

			if got := shouldIssueNewCertificate(
				context.Background(),
				testES,
				tt.args.secret,
				*pod,
				privateKey,
				ca,
				tt.args.rotateBefore,
			); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
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
