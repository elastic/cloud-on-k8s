// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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
	testCluster                  = v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"}}
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
		ObjectMeta: testPod.ObjectMeta,
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
		ObjectMeta: testPod.ObjectMeta,
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
	additionalCA  = [][]byte{[]byte(testAdditionalCA)}
)

const (
	testPemPrivateKey = `
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
	testAdditionalCA = `-----BEGIN CERTIFICATE-----
MIIDKzCCAhOgAwIBAgIRAK7i/u/wsh+i2G0yUygsJckwDQYJKoZIhvcNAQELBQAw
LzEZMBcGA1UECxMQNG1jZnhjbnh0ZjZuNHA5bDESMBAGA1UEAxMJdHJ1c3Qtb25l
MB4XDTE5MDMyMDIwNDg1NloXDTIwMDMxOTIwNDk1NlowLzEZMBcGA1UECxMQNG1j
Znhjbnh0ZjZuNHA5bDESMBAGA1UEAxMJdHJ1c3Qtb25lMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEAu/Pws5FcyJw843pNow/Y95rApWAuGanU99DEmeOG
ggtpc3qtDWWKwLZ6cU+av3u82tf0HYSpy0Z2hn3PS2dGGgHPTr/tTGYA5alu1dn5
CgqQDBVLbkKA1lDcm8w98fRavRw6a0TX5DURqXs+smhdMztQjDNCl3kJ40JbXVAY
x5vhD2pKPCK0VIr9uYK0E/9dvrU0SJGLUlB+CY/DU7c8t22oer2T6fjCZzh3Fhwi
/aOKEwEUoE49orte0N9b1HSKlVePzIUuTTc3UU2ntWi96Uf2FesuAubU11WH4kIL
wRlofty7ewBzVmGte1fKUMjHB3mgb+WYwkEFwjpQL4LhkQIDAQABo0IwQDAOBgNV
HQ8BAf8EBAMCAoQwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMA8GA1Ud
EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAI+qczKQgkb5L5dXzn+KW92J
Sq1rrmaYUYLRTtPFH7t42REPYLs4UV0qR+6v/hJljQbAS+Vu3BioLWuxq85NsIjf
OK1KO7D8lwVI9tAetE0tKILqljTjwZpqfZLZ8fFqwzd9IM/WfoI7Z05k8BSL6XdM
FaRfSe/GJ+DR1dCwnWAVKGxAry4JSceVS9OXxYNRTcfQuT5s8h/6X5UaonTbhil7
91fQFaX8LSuZj23/3kgDTnjPmvj2sz5nODymI4YeTHLjdlMmTufWSJj901ITp7Bw
DMO3GhRADFpMz3vjHA2rHA4AQ6nC8N4lIYTw0AF1VAOC0SDntf6YEgrhRKRFAUY=
-----END CERTIFICATE-----`
)

func init() {
	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCA, err = certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-common-name"},
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
	validatedCertificateTemplate, err = CreateValidatedCertificateTemplate(
		testPod, testCluster, []corev1.Service{testSvc}, testCSR, certificates.DefaultCertValidity)
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
		secret       corev1.Secret
		pod          corev1.Pod
		cluster      v1alpha1.Elasticsearch
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
				pod:          testPod,
				cluster:      testCluster,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "invalid cert data",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: []byte("invalid"),
					},
				},
				pod:          testPod,
				cluster:      testCluster,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "pod name mismatch",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				pod:          corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "different"}},
				cluster:      testCluster,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "pod name mismatch",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				pod:          corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "different"}},
				cluster:      testCluster,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				pod:          testPod,
				cluster:      testCluster,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: false,
		},
		{
			name: "should be rotated soon",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				pod:          testPod,
				cluster:      testCluster,
				rotateBefore: certificates.DefaultCertValidity, // rotate before the same duration as total validity
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIssueNewCertificate(tt.args.cluster, []corev1.Service{testSvc}, tt.args.secret, testCA, tt.args.pod, tt.args.rotateBefore); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doReconcileTransportCertificateSecret(t *testing.T) {
	objMeta := metav1.ObjectMeta{
		Namespace: "namespace",
		Name:      "secret",
	}

	tests := []struct {
		name                            string
		secret                          corev1.Secret
		pod                             corev1.Pod
		additionalTrustedCAsPemEncoded  [][]byte
		wantSecretUpdated               bool
		wantCertUpdateAnnotationUpdated bool
	}{
		{
			name: "Do not requeue without updating secret if there is an additional CA",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CertFileName: pemCert,
					certificates.CSRFileName:  testCSRBytes,
					certificates.CAFileName:   certificates.EncodePEMCert(testCA.Cert.Raw),
				},
			},
			additionalTrustedCAsPemEncoded:  additionalCA,
			pod:                             podWithRunningCertInitializer,
			wantSecretUpdated:               true,
			wantCertUpdateAnnotationUpdated: false,
		},
		{
			name:                            "no cert generated yet: issue one",
			secret:                          corev1.Secret{ObjectMeta: objMeta},
			pod:                             podWithRunningCertInitializer,
			wantSecretUpdated:               true,
			wantCertUpdateAnnotationUpdated: true,
		},
		{
			name:                            "no cert generated, but pod has no IP yet: requeue",
			secret:                          corev1.Secret{ObjectMeta: objMeta},
			pod:                             corev1.Pod{},
			wantSecretUpdated:               false,
			wantCertUpdateAnnotationUpdated: true,
		},
		{
			name:                            "no cert generated, but cert-initializer not running: requeue",
			secret:                          corev1.Secret{ObjectMeta: objMeta},
			pod:                             podWithTerminatedCertInitializer,
			wantSecretUpdated:               false,
			wantCertUpdateAnnotationUpdated: true,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is not running: requeue",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CertFileName: pemCert,
				},
			},
			pod:                             podWithTerminatedCertInitializer,
			wantSecretUpdated:               false,
			wantCertUpdateAnnotationUpdated: true,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is running to serve a new CSR: issue cert",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CertFileName: pemCert,
				},
			},
			pod:                             podWithRunningCertInitializer,
			wantSecretUpdated:               true,
			wantCertUpdateAnnotationUpdated: true,
		},
		{
			name: "a cert already exists, is valid, and cert-initializer is running to serve the same CSR: requeue",
			secret: corev1.Secret{
				ObjectMeta: objMeta,
				Data: map[string][]byte{
					certificates.CertFileName: pemCert,
					certificates.CSRFileName:  testCSRBytes,
					certificates.CAFileName:   certificates.EncodePEMCert(testCA.Cert.Raw),
				},
			},
			pod:                             podWithRunningCertInitializer,
			wantSecretUpdated:               false,
			wantCertUpdateAnnotationUpdated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.WrapClient(fake.NewFakeClient(&tt.secret))
			err := fakeClient.Create(&tt.pod)
			require.NoError(t, err)

			_, err = doReconcileTransportCertificateSecret(
				fakeClient,
				*tt.secret.DeepCopy(), // We need a deepcopy to not update the original data slice
				tt.pod,
				fakeCSRClient,
				testCluster,
				[]corev1.Service{testSvc},
				testCA, tt.additionalTrustedCAsPemEncoded,
				certificates.DefaultCertValidity,
				certificates.DefaultRotateBefore,
			)
			require.NoError(t, err)

			var updatedSecret corev1.Secret
			err = fakeClient.Get(k8s.ExtractNamespacedName(&objMeta), &updatedSecret)
			require.NoError(t, err)

			var updatedPod corev1.Pod
			err = fakeClient.Get(k8s.ExtractNamespacedName(&tt.pod), &updatedPod)

			isUpdated := !reflect.DeepEqual(tt.secret, updatedSecret)
			require.Equal(t, tt.wantSecretUpdated, isUpdated)
			if tt.wantSecretUpdated {
				assert.NotEmpty(t, updatedSecret.Data[certificates.CAFileName])
				assert.NotEmpty(t, updatedSecret.Data[certificates.CSRFileName])
				assert.NotEmpty(t, updatedSecret.Data[certificates.CertFileName])
				if tt.wantCertUpdateAnnotationUpdated {
					// check that the CSR annotation has been updated
					assert.NotEmpty(t, updatedSecret.Annotations[LastCSRUpdateAnnotation])
					lastCertUpdate, err := time.Parse(time.RFC3339, updatedSecret.Annotations[LastCSRUpdateAnnotation])
					require.NoError(t, err)
					assert.True(t, lastCertUpdate.Add(-5*time.Minute).Before(time.Now()))
					// also check that the pod annotation has been updated too
					assert.NotEmpty(t, updatedPod.Annotations[annotation.UpdateAnnotation])
					lastPodUpdate, err := time.Parse(time.RFC3339Nano, updatedPod.Annotations[annotation.UpdateAnnotation])
					require.NoError(t, err)
					assert.True(t, lastPodUpdate.Add(-5*time.Minute).Before(time.Now()))
				}
			}
		})
	}
}
