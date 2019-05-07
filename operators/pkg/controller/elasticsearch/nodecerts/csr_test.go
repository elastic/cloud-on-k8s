// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package nodecerts

import (
	"crypto/x509"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
)

type FakeCSRClient struct {
	csr []byte
}

func (f FakeCSRClient) RetrieveCSR(pod corev1.Pod) ([]byte, error) {
	return f.csr, nil
}

// roundTripSerialize does a serialization round-trip of the certificate in order to make sure any extra extensions
// are parsed and considered part of the certificate
func roundTripSerialize(cert *certificates.ValidatedCertificateTemplate) (*x509.Certificate, error) {
	certData, err := testCA.CreateCertificate(*cert)
	if err != nil {
		return nil, err
	}
	certRT, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return certRT, nil
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

func Test_createValidatedCertificateTemplate(t *testing.T) {
	validatedCert, err := CreateValidatedCertificateTemplate(testPod, testCluster, []corev1.Service{testSvc}, testCSR, certificates.DefaultCertValidity)
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
	otherNames, err := certificates.ParseSANGeneralNamesOtherNamesOnly(certRT)
	require.NoError(t, err)

	// we expect this name to be used for both the common name as well as the es othername
	cn := "test-pod-name.node.test-es-name.test-namespace.es.cluster.local"

	otherName, err := (&certificates.UTF8StringValuedOtherName{
		OID:   certificates.CommonNameObjectIdentifier,
		Value: cn,
	}).ToOtherName()
	require.NoError(t, err)

	assert.Equal(t, certRT.Subject.CommonName, cn)
	assert.Contains(t, otherNames, certificates.GeneralName{OtherName: *otherName})
}

func Test_buildGeneralNames(t *testing.T) {
	expectedCommonName := "test-pod-name.node.test-es-name.test-namespace.es.cluster.local"
	otherName, err := (&certificates.UTF8StringValuedOtherName{
		OID:   certificates.CommonNameObjectIdentifier,
		Value: expectedCommonName,
	}).ToOtherName()
	require.NoError(t, err)
	sanDNS1 := "my.dns.com"
	sanDNS2 := "my.second.dns.com"
	sanIP1 := "4.4.6.7"
	sanIPv6 := "2001:db8:0:85a3:0:0:ac1f:8001"
	type args struct {
		cluster v1alpha1.Elasticsearch
		svcs    []corev1.Service
		pod     corev1.Pod
	}
	tests := []struct {
		name    string
		args    args
		want    []certificates.GeneralName
		wantErr bool
	}{
		{
			name: "no svcs and user-provided SANs",
			args: args{
				cluster: testCluster,
				pod:     testPod,
			},
			want: []certificates.GeneralName{
				{OtherName: *otherName},
				{DNSName: expectedCommonName},
				{DNSName: testPod.Name},
				{IPAddress: net.ParseIP(testIP).To4()},
				{IPAddress: net.ParseIP("127.0.0.1").To4()},
			},
		},
		{
			name: "with svcs and user-provided SANs",
			args: args{
				cluster: v1alpha1.Elasticsearch{
					ObjectMeta: testCluster.ObjectMeta,
					Spec: v1alpha1.ElasticsearchSpec{
						HTTP: commonv1alpha1.HTTPConfig{
							TLS: commonv1alpha1.TLSOptions{
								SelfSignedCertificate: &commonv1alpha1.SelfSignedCertificate{
									SubjectAlternativeNames: []commonv1alpha1.SubjectAlternativeName{
										{
											DNS: sanDNS1,
										},
										{
											DNS: sanDNS2,
										},
										{
											IP: sanIP1,
										},
										{
											IP: sanIPv6,
										},
									},
								},
							},
						},
					},
				},
				svcs: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "svc-namespace",
							Name:      "svc-name",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.11.12.13",
						},
					},
				},
				pod: testPod,
			},
			want: []certificates.GeneralName{
				{OtherName: *otherName},
				{DNSName: expectedCommonName},
				{DNSName: testPod.Name},
				{IPAddress: net.ParseIP(testIP).To4()},
				{IPAddress: net.ParseIP("127.0.0.1").To4()},
				{DNSName: "my.dns.com"},
				{DNSName: "my.second.dns.com"},
				{IPAddress: net.ParseIP(sanIP1).To4()},
				{IPAddress: net.ParseIP(sanIPv6)},
				{IPAddress: net.ParseIP("10.11.12.13").To4()},
				{DNSName: "svc-name"},
				{DNSName: "svc-name.svc-namespace.svc.cluster.local"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildGeneralNames(tt.args.cluster, tt.args.svcs, tt.args.pod)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
