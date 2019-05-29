// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	netutil "github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
)

// CSRRequestDelay limits the number of CSR requests we do in consecutive reconciliations
const CSRRequestDelay = 1 * time.Minute

// maybeRequestCSR requests the pod for a new CSR if required, or returns nil.
func maybeRequestCSR(pod corev1.Pod, csrClient certificates.CSRClient, lastCSRUpdate string) ([]byte, error) {
	// If the CSR secret was updated very recently, chances are we already issued a new certificate
	// which has not yet been propagated to the pod (it can take more than 1 minute).
	// In such case, there is no need to request the same CSR again and again at each reconciliation.
	lastUpdate, err := time.Parse(time.RFC3339, lastCSRUpdate)
	if err != nil {
		log.V(1).Info("lastCSRUpdate time cannot be parsed, probably because not set yet. Ignoring.", "pod", pod.Name)
	} else {
		delay := time.Since(lastUpdate)
		if delay > 0 && delay < CSRRequestDelay {
			log.V(1).Info("CSR was already updated recently, let's wait before requesting a new one", "pod", pod.Name)
			return nil, nil
		}
	}
	// Check status of the pod's cert-initializer init container: if running, it's waiting for
	// a valid certificate to be issued, hence we should request a new CSR.
	for _, c := range pod.Status.InitContainerStatuses {
		if c.Name == initcontainer.CertInitializerContainerName && c.State.Running != nil {
			newCSR, err := csrClient.RetrieveCSR(pod)
			if err != nil && err != io.EOF { // EOF is ok, just the cert-initializer shutting down
				return nil, err
			}
			return newCSR, nil
		}
	}
	return nil, nil
}

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
// this needs to be kept in sync with the usage of trust_restrictions (see elasticsearch.TrustConfig)
func buildCertificateCommonName(pod corev1.Pod, clusterName, namespace string) string {
	return fmt.Sprintf("%s.node.%s.%s.es.cluster.local", pod.Name, clusterName, namespace)
}
