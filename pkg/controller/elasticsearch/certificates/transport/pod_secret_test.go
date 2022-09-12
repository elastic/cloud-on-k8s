// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
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
