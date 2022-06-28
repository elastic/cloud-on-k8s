// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	netutil "github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

// createValidatedCertificateTemplate validates a CSR and creates a certificate template.
func createValidatedCertificateTemplate(
	pod corev1.Pod,
	cluster esv1.Elasticsearch,
	csr *x509.CertificateRequest,
	certValidity time.Duration,
) (*certificates.ValidatedCertificateTemplate, error) {
	if err := csr.CheckSignature(); err != nil {
		return nil, err
	}

	generalNames, err := buildGeneralNames(cluster, pod)
	if err != nil {
		return nil, err
	}

	generalNamesBytes, err := certificates.MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		return nil, err
	}

	certificateTemplate := certificates.ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         buildCertificateCommonName(pod, cluster),
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
	cluster esv1.Elasticsearch,
	pod corev1.Pod,
) ([]certificates.GeneralName, error) {
	podIP := net.ParseIP(pod.Status.PodIP)
	if podIP == nil {
		return nil, errors.Errorf("pod currently has no valid IP, found: [%s]", pod.Status.PodIP)
	}

	ssetName := pod.Labels[label.StatefulSetNameLabelName]
	svcName := nodespec.HeadlessServiceName(ssetName)

	commonName := buildCertificateCommonName(pod, cluster)

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
		// add the transport service name for remote cluster connections initially connecting through the service
		// the DNS name has to match the seed hosts configured in the remote cluster settings
		{DNSName: fmt.Sprintf("%s.%s.svc", esv1.TransportService(cluster.Name), cluster.Namespace)},
		// add the resolvable DNS name of the Pod as published by Elasticsearch
		{DNSName: fmt.Sprintf("%s.%s", pod.Name, svcName)},
		{IPAddress: netutil.IPToRFCForm(podIP)},
		{IPAddress: netutil.IPToRFCForm(netutil.LoopbackFor(netutil.ToIPFamily(podIP.String())))},
	}

	for _, san := range cluster.Spec.Transport.TLS.SubjectAlternativeNames {
		if san.DNS != "" {
			generalNames = append(generalNames, certificates.GeneralName{DNSName: san.DNS})
		}
		if san.IP != "" {
			generalNames = append(generalNames, certificates.GeneralName{IPAddress: netutil.IPToRFCForm(net.ParseIP(san.IP))})
		}
	}
	return generalNames, nil
}

// buildCertificateCommonName returns the CN (and ES otherName) entry for a given Elasticsearch Pod.
// If the user provided an otherName suffix in the spec, it prepends the pod name to it (<pod_name>.<user-suffix).
// Otherwise, it defaults to <pod_name>.node.<es_name>.es.local.
func buildCertificateCommonName(pod corev1.Pod, es esv1.Elasticsearch) string {
	userConfiguredSuffix := es.Spec.Transport.TLS.OtherNameSuffix
	if userConfiguredSuffix == "" {
		return fmt.Sprintf("%s.node.%s.%s.es.local", pod.Name, es.Name, es.Namespace)
	}
	return fmt.Sprintf("%s.%s", pod.Name, userConfiguredSuffix)
}
