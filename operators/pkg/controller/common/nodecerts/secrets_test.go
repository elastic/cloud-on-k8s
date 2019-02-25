// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	cryptorand "crypto/rand"
	"crypto/x509"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts/certutil"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fixtures
var (
	testCSRBytes                 []byte
	testCSR                      *x509.CertificateRequest
	validatedCertificateTemplate *ValidatedCertificateTemplate
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

type FakeCSRClient struct {
	csr []byte
}

func (f FakeCSRClient) RetrieveCSR(pod corev1.Pod) ([]byte, error) {
	return f.csr, nil
}

func init() {
	initTestVars()
	var err error
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
	pemCert = certutil.EncodePEMCert(certData, testCa.Cert.Raw)
}

func Test_createValidatedCertificateTemplate(t *testing.T) {
	validatedCert, err := CreateValidatedCertificateTemplate(testPod, "test-es-name", "test-namespace", []corev1.Service{testSvc}, testCSR)
	require.NoError(t, err)

	// roundtrip the certificate
	certRT, err := roundTripSerialize(validatedCert)
	require.NoError(t, err)
	require.NotNil(t, certRT, "roundtripped certificate should not be nil")

	// regular dns names and ip addresses should be present in the cert
	assert.Contains(t, certRT.DNSNames, testPod.Name)
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(testIP).To4())
	assert.Contains(t, certRT.IPAddresses, net.ParseIP("127.0.0.1").To4())

	// service ip and hosts should be present in the cert
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(testSvc.Spec.ClusterIP).To4())
	assert.Contains(t, certRT.DNSNames, testSvc.Name)
	assert.Contains(t, certRT.DNSNames, getServiceFullyQualifiedHostname(testSvc))

	// es specific othernames is a bit more difficult to get to, but should be present:
	otherNames, err := certutil.ParseSANGeneralNamesOtherNamesOnly(certRT)
	require.NoError(t, err)

	// we expect this name to be used for both the common name as well as the es othername
	cn := "test-pod-name.node.test-es-name.test-namespace.es.cluster.local"

	otherName, err := (&certutil.UTF8StringValuedOtherName{
		OID:   certutil.CommonNameObjectIdentifier,
		Value: cn,
	}).ToOtherName()
	require.NoError(t, err)

	assert.Equal(t, certRT.Subject.CommonName, cn)
	assert.Contains(t, otherNames, certutil.GeneralName{OtherName: *otherName})
}

// roundTripSerialize does a serialization round-trip of the certificate in order to make sure any extra extensions
// are parsed and considered part of the certificate
func roundTripSerialize(cert *ValidatedCertificateTemplate) (*x509.Certificate, error) {
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
			if got := shouldIssueNewCertificate(tt.args.secret, testCa, tt.args.pod); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_maybeRequestCSR(t *testing.T) {
	tests := []struct {
		name          string
		lastCSRUpdate string
		pod           corev1.Pod
		want          []byte
		wantErr       bool
	}{
		{
			name:          "last request was made a day ago, should request a new CSR",
			lastCSRUpdate: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			pod:           podWithRunningCertInitializer,
			want:          fakeCSRClient.csr,
		},
		{
			name:          "last request was made very recently, should not request a new CSR",
			lastCSRUpdate: time.Now().Add(-5 * time.Second).Format(time.RFC3339),
			pod:           podWithRunningCertInitializer,
			want:          nil,
		},
		{
			name:          "last request time isn't set, should request a new CSR",
			lastCSRUpdate: "",
			pod:           podWithRunningCertInitializer,
			want:          fakeCSRClient.csr,
		},
		{
			name:          "last request time has the wrong format, request a new CSR",
			lastCSRUpdate: "yolo",
			pod:           podWithRunningCertInitializer,
			want:          fakeCSRClient.csr,
		},
		{
			name:          "last request time is in the future, should request a new CSR",
			lastCSRUpdate: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			pod:           podWithRunningCertInitializer,
			want:          fakeCSRClient.csr,
		},
		{
			name:          "cert-initializer is terminated, should not request a CSR",
			lastCSRUpdate: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			pod:           podWithTerminatedCertInitializer,
			want:          nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := maybeRequestCSR(tt.pod, fakeCSRClient, tt.lastCSRUpdate)
			if (err != nil) != tt.wantErr {
				t.Errorf("maybeRequestCSR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("maybeRequestCSR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileNodeCertificateSecret(t *testing.T) {
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

			_, err := ReconcileNodeCertificateSecret(fakeClient, tt.secret, tt.pod, fakeCSRClient, "cluster", "namespace", []corev1.Service{testSvc}, testCa, nil)
			require.NoError(t, err)

			var updatedSecret corev1.Secret
			err = fakeClient.Get(k8s.ExtractNamespacedName(&objMeta), &updatedSecret)
			require.NoError(t, err)

			isUpdated := !reflect.DeepEqual(tt.secret, updatedSecret)
			require.Equal(t, tt.wantSecretUpdated, isUpdated)
			if tt.wantSecretUpdated {
				assert.NotEmpty(t, updatedSecret.Annotations[LastCSRUpdateAnnotation])
				assert.NotEmpty(t, updatedSecret.Data[CAFileName])
				assert.NotEmpty(t, updatedSecret.Data[CSRFileName])
				assert.NotEmpty(t, updatedSecret.Data[CertFileName])
			}
		})
	}
}
