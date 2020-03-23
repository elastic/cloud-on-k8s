// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
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
						PodCertFileName(testPod.Name): pemCert,
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
						PodCertFileName(testPod.Name): pemCert,
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
						PodCertFileName(testPod.Name): pemCert,
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
				testES,
				tt.args.secret,
				*tt.args.pod,
				testRSAPrivateKey,
				testCA,
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
					PodCertFileName(testPod.Name): pemCert,
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
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
					PodKeyFileName(testPod.Name): certificates.EncodePEMPrivateKey(*testRSAPrivateKey),
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
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
					PodKeyFileName(testPod.Name):  certificates.EncodePEMPrivateKey(*testRSAPrivateKey),
					PodCertFileName(testPod.Name): certificates.EncodePEMCert(testCA.Cert.Raw),
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
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
					PodKeyFileName(testPod.Name):  certificates.EncodePEMPrivateKey(*testRSAPrivateKey),
					PodCertFileName(testPod.Name): []byte("invalid"),
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
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
					PodKeyFileName(testPod.Name):  certificates.EncodePEMPrivateKey(*testRSAPrivateKey),
					PodCertFileName(testPod.Name): pemCert,
				},
			},
			assertions: func(t *testing.T, before corev1.Secret, after corev1.Secret) {
				assert.Equal(t, before, after)
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
				testES,
				tt.secret,
				*tt.pod,
				testCA,
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
