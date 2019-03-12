// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package nodecerts

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fixtures
var (
	testCA                       *certificates.CA
	testRSAPrivateKey            *rsa.PrivateKey
	testCSRBytes                 []byte
	testCSR                      *x509.CertificateRequest
	validatedCertificateTemplate *certificates.ValidatedCertificateTemplate
	certData                     []byte
	pemCert                      []byte
	testIP                       = "1.2.3.4"
	testPod                      = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod-name",
		},
		Status: corev1.PodStatus{
			PodIP: testIP,
		},
	}
	testSvc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "2.2.3.3",
		},
	}
	podWithRunningCertInitializer = corev1.Pod{
		Status: corev1.PodStatus{
			PodIP: testIP,
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name: initcontainer.CertInitializerContainerName,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	podWithTerminatedCertInitializer = corev1.Pod{
		Status: corev1.PodStatus{
			PodIP: testIP,
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name: initcontainer.CertInitializerContainerName,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
				},
			},
		},
	}
	fakeCSRClient FakeCSRClient
)

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

func init() {
	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCA, err = certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		CommonName: "test",
		PrivateKey: testRSAPrivateKey,
	}); err != nil {
		panic("Failed to create new self signed CA: " + err.Error())
	}
	testCSRBytes, err = x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, testRSAPrivateKey)
	if err != nil {
		panic("Failed to create CSR:" + err.Error())
	}
	fakeCSRClient = FakeCSRClient{csr: testCSRBytes}
	testCSR, err = x509.ParseCertificateRequest(testCSRBytes)
	validatedCertificateTemplate, err = CreateValidatedCertificateTemplate(testPod, "test-es-name", "test-namespace", []corev1.Service{testSvc}, testCSR)
	if err != nil {
		panic("Failed to create validated cert template:" + err.Error())
	}
	certData, err = testCA.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}
	pemCert = certificates.EncodePEMCert(certData, testCA.Cert.Raw)
}

func Test_shouldIssueNewCertificate(t *testing.T) {
	type args struct {
		secret corev1.Secret
		pod    corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "missing cert in secret",
			args: args{
				secret: corev1.Secret{},
				pod:    testPod,
			},
			want: true,
		},
		{
			name: "invalid cert data",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: []byte("invalid"),
					},
				},
				pod: testPod,
			},
			want: true,
		},
		{
			name: "pod name mismatch",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "different"}},
			},
			want: true,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				pod: testPod,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIssueNewCertificate(tt.args.secret, testCA, tt.args.pod); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doReconcile(t *testing.T) {
	objMeta := metav1.ObjectMeta{
		Namespace: "namespace",
		Name:      "secret",
	}
	tests := []struct {
		name              string
		secret            corev1.Secret
		pod               corev1.Pod
		wantSecretUpdated bool
	}{
		{
			name:              "no cert generated yet: issue one",
			secret:            corev1.Secret{ObjectMeta: objMeta},
			pod:               podWithRunningCertInitializer,
			wantSecretUpdated: true,
		},
		{
			name:              "no cert generated, but pod has no IP yet: requeue",
			secret:            corev1.Secret{ObjectMeta: objMeta},
			pod:               corev1.Pod{},
			wantSecretUpdated: false,
		},
		{
			name:              "no cert generated, but cert-initializer not running: requeue",
			secret:            corev1.Secret{ObjectMeta: objMeta},
			pod:               podWithTerminatedCertInitializer,
			wantSecretUpdated: false,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is not running: requeue",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CertFileName: pemCert,
				},
			},
			pod:               podWithTerminatedCertInitializer,
			wantSecretUpdated: false,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is running to serve a new CSR: issue cert",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CertFileName: pemCert,
				},
			},
			pod:               podWithRunningCertInitializer,
			wantSecretUpdated: true,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is running to serve the same CSR: requeue",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					CertFileName: pemCert,
					CSRFileName:  testCSRBytes,
				},
			},
			pod:               podWithRunningCertInitializer,
			wantSecretUpdated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.WrapClient(fake.NewFakeClient(&tt.secret))
			err := fakeClient.Create(&tt.pod)
			require.NoError(t, err)

			_, err = doReconcile(fakeClient, tt.secret, tt.pod, fakeCSRClient, "cluster", "namespace", []corev1.Service{testSvc}, testCA, nil)
			require.NoError(t, err)

			var updatedSecret corev1.Secret
			err = fakeClient.Get(k8s.ExtractNamespacedName(&objMeta), &updatedSecret)
			require.NoError(t, err)

			var updatedPod corev1.Pod
			err = fakeClient.Get(k8s.ExtractNamespacedName(&tt.pod), &updatedPod)

			isUpdated := !reflect.DeepEqual(tt.secret, updatedSecret)
			require.Equal(t, tt.wantSecretUpdated, isUpdated)
			if tt.wantSecretUpdated {
				assert.NotEmpty(t, updatedSecret.Annotations[LastCSRUpdateAnnotation])
				assert.NotEmpty(t, updatedSecret.Data[certificates.CAFileName])
				assert.NotEmpty(t, updatedSecret.Data[CSRFileName])
				assert.NotEmpty(t, updatedSecret.Data[CertFileName])
				lastCertUpdate, err := time.Parse(time.RFC3339, updatedPod.Annotations[lastCertUpdateAnnotation])
				require.NoError(t, err)
				assert.True(t, lastCertUpdate.Add(-5*time.Minute).Before(time.Now()))
			}
		})
	}
}
