// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
)

// roundTripSerialize does a serialization round-trip of the certificate in order to make sure any extra extensions
// are parsed and considered part of the certificate
func roundTripSerialize(cert *certificates.ValidatedCertificateTemplate) (*x509.Certificate, error) {
	certData, err := testRSACA.CreateCertificate(*cert)
	if err != nil {
		return nil, err
	}
	certRT, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return certRT, nil
}

func Test_createValidatedCertificateTemplate(t *testing.T) {
	// we expect this name to be used for both the common name as well as the es othername
	cn := "test-pod-name.node.test-es-name.test-namespace.es.local"

	validatedCert, err := createValidatedCertificateTemplate(testPod, testES, testRSACSR, certificates.DefaultCertValidity)
	require.NoError(t, err)

	// roundtrip the certificate
	certRT, err := roundTripSerialize(validatedCert)
	require.NoError(t, err)
	require.NotNil(t, certRT, "roundtripped certificate should not be nil")

	// regular dns names and ip addresses should be present in the cert
	assert.Contains(t, certRT.DNSNames, cn)
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(testIP).To4())
	assert.Contains(t, certRT.IPAddresses, net.ParseIP("127.0.0.1").To4())

	// es specific othernames is a bit more difficult to get to, but should be present:
	otherNames, err := certificates.ParseSANGeneralNamesOtherNamesOnly(certRT)
	require.NoError(t, err)

	otherName, err := (&certificates.UTF8StringValuedOtherName{
		OID:   certificates.CommonNameObjectIdentifier,
		Value: cn,
	}).ToOtherName()
	require.NoError(t, err)

	assert.Equal(t, certRT.Subject.CommonName, cn)
	assert.Contains(t, otherNames, certificates.GeneralName{OtherName: *otherName})
}

func Test_buildGeneralNames(t *testing.T) {
	expectedCommonName := "test-pod-name.node.test-es-name.test-namespace.es.local"
	expectedTransportSvcName := "test-es-name-es-transport.test-namespace.svc"

	mkOtherName := func(name string) certificates.OtherName {
		otherName, err := (&certificates.UTF8StringValuedOtherName{
			OID:   certificates.CommonNameObjectIdentifier,
			Value: name,
		}).ToOtherName()
		require.NoError(t, err)
		return *otherName
	}

	type args struct {
		cluster esv1.Elasticsearch
		pod     corev1.Pod
	}
	expectedGeneralNames := []certificates.GeneralName{
		{OtherName: mkOtherName(expectedCommonName)},
		{DNSName: expectedCommonName},
		{DNSName: expectedTransportSvcName},
		{DNSName: "test-pod-name.test-sset"},
		{IPAddress: net.ParseIP(testIP).To4()},
		{IPAddress: net.ParseIP("127.0.0.1").To4()},
	}
	tests := []struct {
		name string
		args args
		want []certificates.GeneralName
	}{
		{
			name: "no svcs and user-provided SANs by default",
			args: args{
				cluster: testES,
				pod:     testPod,
			},
			want: expectedGeneralNames,
		},
		{
			name: "optional user provided SANs",
			args: args{
				cluster: func() esv1.Elasticsearch {
					es := testES
					es.Spec.Transport.TLS.SubjectAlternativeNames = []commonv1.SubjectAlternativeName{
						{
							DNS: "my-custom-domain",
							IP:  "111.222.333.444",
						},
					}
					return es
				}(),
				pod: testPod,
			},
			want: append(expectedGeneralNames, []certificates.GeneralName{
				{DNSName: "my-custom-domain"},
				{IPAddress: net.ParseIP("111.222.333.444").To4()},
			}...),
		},
		{
			name: "optional user provided SANs",
			args: args{
				cluster: func() esv1.Elasticsearch {
					es := testES
					es.Spec.Transport.TLS.SubjectAlternativeNames = []commonv1.SubjectAlternativeName{
						{
							DNS: "my-custom-domain",
						},
						{
							IP: "1.2.3.4",
						},
					}
					return es
				}(),
				pod: testPod,
			},
			want: append(expectedGeneralNames, []certificates.GeneralName{
				{DNSName: "my-custom-domain"},
				{IPAddress: net.ParseIP("1.2.3.4").To4()},
			}...),
		},
		{
			name: "optional user provided SANs",
			args: args{
				cluster: func() esv1.Elasticsearch {
					es := testES
					es.Spec.Transport.TLS.SubjectAlternativeNames = []commonv1.SubjectAlternativeName{
						{
							IP: "1.2.3.4",
						},
					}
					return es
				}(),
				pod: testPod,
			},
			want: append(expectedGeneralNames, []certificates.GeneralName{
				{IPAddress: net.ParseIP("1.2.3.4").To4()},
			}...),
		},
		{
			name: "optional user provided SANs",
			args: args{
				cluster: func() esv1.Elasticsearch {
					es := testES
					es.Spec.Transport.TLS.SubjectAlternativeNames = []commonv1.SubjectAlternativeName{
						{
							DNS: "my-custom-domain",
						},
					}
					return es
				}(),
				pod: testPod,
			},
			want: append(expectedGeneralNames, []certificates.GeneralName{
				{DNSName: "my-custom-domain"},
			}...),
		},
		{
			name: "custom name suffix",
			args: args{
				cluster: func() esv1.Elasticsearch {
					es := testES
					es.Spec.Transport.TLS.OtherNameSuffix = "user.provided.suffix"
					return es
				}(),
				pod: testPod,
			},
			want: func() []certificates.GeneralName {
				expectedCommonName := "test-pod-name.user.provided.suffix"
				return []certificates.GeneralName{
					{OtherName: mkOtherName(expectedCommonName)},
					{DNSName: expectedCommonName},
					{DNSName: expectedTransportSvcName},
					{DNSName: "test-pod-name.test-sset"},
					{IPAddress: net.ParseIP(testIP).To4()},
					{IPAddress: net.ParseIP("127.0.0.1").To4()},
				}
			}(),
		},
		{
			name: "custom name suffix with additional SANs",
			args: args{
				cluster: func() esv1.Elasticsearch {
					es := testES
					es.Spec.Transport.TLS.OtherNameSuffix = "user.provided.suffix"
					es.Spec.Transport.TLS.SubjectAlternativeNames = []commonv1.SubjectAlternativeName{
						{
							DNS: "my-custom-domain",
						},
					}
					return es
				}(),
				pod: testPod,
			},
			want: func() []certificates.GeneralName {
				expectedCommonName := "test-pod-name.user.provided.suffix"
				return []certificates.GeneralName{
					{OtherName: mkOtherName(expectedCommonName)},
					{DNSName: expectedCommonName},
					{DNSName: expectedTransportSvcName},
					{DNSName: "test-pod-name.test-sset"},
					{IPAddress: net.ParseIP(testIP).To4()},
					{IPAddress: net.ParseIP("127.0.0.1").To4()},
					{DNSName: "my-custom-domain"},
				}
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildGeneralNames(tt.args.cluster, tt.args.pod)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
