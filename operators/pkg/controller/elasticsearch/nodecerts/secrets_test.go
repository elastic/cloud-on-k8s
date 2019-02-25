// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fixtures
var (
	testCa                       *certificates.Ca
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

type FakeCSRClient struct {
	csr []byte
}

func (f FakeCSRClient) RetrieveCSR(pod corev1.Pod) ([]byte, error) {
	return f.csr, nil
}

func init() {
	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCa, err = certificates.NewSelfSignedCaUsingKey("test", testRSAPrivateKey); err != nil {
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
	certData, err = testCa.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}
	pemCert = certificates.EncodePEMCert(certData, testCa.Cert.Raw)
}

// roundTripSerialize does a serialization round-trip of the certificate in order to make sure any extra extensions
// are parsed and considered part of the certificate
func roundTripSerialize(cert *certificates.ValidatedCertificateTemplate) (*x509.Certificate, error) {
	certData, err := testCa.CreateCertificate(*cert)
	if err != nil {
		return nil, err
	}
	certRT, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return certRT, nil
}

func TestEnsureNodeCertificateSecretExists(t *testing.T) {
	stubOwner := &corev1.Pod{}
	preExistingSecret := &corev1.Secret{}

	type args struct {
		c                   k8s.Client
		scheme              *runtime.Scheme
		owner               metav1.Object
		pod                 corev1.Pod
		nodeCertificateType string
		labels              map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    func(*testing.T, *corev1.Secret)
		wantErr bool
	}{
		{
			name: "should create a secret if it does not already exist",
			args: args{
				c:                   k8s.WrapClient(fake.NewFakeClient()),
				nodeCertificateType: LabelNodeCertificateTypeElasticsearchAll,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, secret.Labels, LabelNodeCertificateType)
				assert.Equal(t, secret.Labels[LabelNodeCertificateType], LabelNodeCertificateTypeElasticsearchAll)
			},
		},
		{
			name: "should not create a new secret if it already exists",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(preExistingSecret)),
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				assert.Equal(t, preExistingSecret, secret)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.scheme == nil {
				tt.args.scheme = scheme.Scheme
			}

			if tt.args.owner == nil {
				tt.args.owner = stubOwner
			}

			got, err := EnsureNodeCertificateSecretExists(tt.args.c, tt.args.scheme, tt.args.owner, tt.args.pod, tt.args.nodeCertificateType, tt.args.labels)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureNodeCertificateSecretExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, got)
		})
	}
}
