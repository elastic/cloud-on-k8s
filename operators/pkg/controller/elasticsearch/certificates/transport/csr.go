// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	netutil "github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// CreateValidatedCertificateTemplate validates a CSR and creates a certificate template.
func CreateValidatedCertificateTemplate(
	pod corev1.Pod,
	cluster v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	csr *x509.CertificateRequest,
	certValidity time.Duration,
) (*certificates.ValidatedCertificateTemplate, error) {
	generalNames, err := buildGeneralNames(cluster, svcs, pod)
	if err != nil {
		return nil, err
	}

	generalNamesBytes, err := certificates.MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		return nil, err
	}

	// TODO: csr signature is not checked
	certificateTemplate := certificates.ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         buildCertificateCommonName(pod, cluster.Name, cluster.Namespace),
			OrganizationalUnit: []string{cluster.Name},
		},

		ExtraExtensions: []pkix.Extension{
			{Id: certificates.SubjectAlternativeNamesObjectIdentifier, Value: generalNamesBytes},
		},
		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(certValidity),

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})

	return &certificateTemplate, nil
}

func buildGeneralNames(
	cluster v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	pod corev1.Pod,
) ([]certificates.GeneralName, error) {
	podIP := net.ParseIP(pod.Status.PodIP)
	if podIP == nil {
		return nil, fmt.Errorf("pod currently has no valid IP, found: [%s]", pod.Status.PodIP)
	}

	commonName := buildCertificateCommonName(pod, cluster.Name, cluster.Namespace)

	commonNameUTF8OtherName := &certificates.UTF8StringValuedOtherName{
		OID:   certificates.CommonNameObjectIdentifier,
		Value: commonName,
	}
	commonNameOtherName, err := commonNameUTF8OtherName.ToOtherName()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create othername")
	}

	// because we're using the ES-customized subject alternative-names extension, we have to handle all the general
	// names here instead of using x509.Certificate.DNSNames, .IPAddresses etc.
	generalNames := []certificates.GeneralName{
		{OtherName: *commonNameOtherName},
		{DNSName: commonName},
		{IPAddress: netutil.MaybeIPTo4(podIP)},
		{IPAddress: net.ParseIP("127.0.0.1").To4()},
	}

	return generalNames, nil
}

// buildCertificateCommonName returns the CN (and ES othername) entry for a given pod within a stack
func buildCertificateCommonName(pod corev1.Pod, clusterName, namespace string) string {
	return fmt.Sprintf("%s.node.%s.%s.es.local", pod.Name, clusterName, namespace)
}
