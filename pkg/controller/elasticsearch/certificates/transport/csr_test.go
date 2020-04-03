// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
)

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

func Test_createValidatedCertificateTemplate(t *testing.T) {
	// we expect this name to be used for both the common name as well as the es othername
	cn := "test-pod-name.node.test-es-name.test-namespace.es.local"

	validatedCert, err := createValidatedCertificateTemplate(
		testPod, testES, testCSR, certificates.DefaultCertValidity,
	)
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
	otherName, err := (&certificates.UTF8StringValuedOtherName{
		OID:   certificates.CommonNameObjectIdentifier,
		Value: expectedCommonName,
	}).ToOtherName()
	require.NoError(t, err)

	type args struct {
		cluster esv1.Elasticsearch
		pod     corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want []certificates.GeneralName
	}{
		{
			name: "no svcs and user-provided SANs",
			args: args{
				cluster: testES,
				pod:     testPod,
			},
			want: []certificates.GeneralName{
				{OtherName: *otherName},
				{DNSName: expectedCommonName},
				{DNSName: expectedTransportSvcName},
				{IPAddress: net.ParseIP(testIP).To4()},
				{IPAddress: net.ParseIP("127.0.0.1").To4()},
			},
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
