// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	corev1 "k8s.io/api/core/v1"
)

// PodKeyFileName returns the name of the private key entry for a specific pod in a transport certificates secret.
func PodKeyFileName(podName string) string {
	return fmt.Sprintf("%s.%s", podName, certificates.KeyFileName)
}

// PodCertFileName returns the name of the certificates entry for a specific pod in a transport certificates secret.
func PodCertFileName(podName string) string {
	return fmt.Sprintf("%s.%s", podName, certificates.CertFileName)
}

// ensureTransportCertificatesSecretContentsForPod ensures that the transport certificates secret has the correct
// content for a specific pod
func ensureTransportCertificatesSecretContentsForPod(
	es v1alpha1.Elasticsearch,
	secret *corev1.Secret,
	pod corev1.Pod,
	ca *certificates.CA,
	rotationParams certificates.RotationParams,
) error {
	// verify that the secret contains a parsable private key, create if it does not exist
	var privateKey *rsa.PrivateKey
	needsNewPrivateKey := true
	if privateKeyData, ok := secret.Data[PodKeyFileName(pod.Name)]; ok {
		storedPrivateKey, err := certificates.ParsePEMPrivateKey(privateKeyData)
		if err != nil {
			log.Error(err, "Unable to parse stored private key",
				"namespace", pod.Namespace, "pod_name", pod.Name)
		} else {
			needsNewPrivateKey = false
			privateKey = storedPrivateKey
		}
	}

	// if we need a new private key, generate it
	if needsNewPrivateKey {
		generatedPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return err
		}

		privateKey = generatedPrivateKey
		secret.Data[PodKeyFileName(pod.Name)] = certificates.EncodePEMPrivateKey(*privateKey)
	}

	if shouldIssueNewCertificate(es, *secret, pod, privateKey, ca, rotationParams.RotateBefore) {
		log.Info(
			"Issuing new certificate",
			"pod_name", pod.Name,
		)

		csr, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, privateKey)
		if err != nil {
			return err
		}

		// create a cert from the csr
		parsedCSR, err := x509.ParseCertificateRequest(csr)
		if err != nil {
			return err
		}

		validatedCertificateTemplate, err := createValidatedCertificateTemplate(
			pod, es, parsedCSR, rotationParams.Validity,
		)
		if err != nil {
			return err
		}
		// sign the certificate
		certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return err
		}

		// store the issued certificate in a secret mounted into the pod
		secret.Data[PodCertFileName(pod.Name)] = certificates.EncodePEMCert(certData, ca.Cert.Raw)
	}

	return nil
}

// shouldIssueNewCertificate returns true if we should issue a new certificate.
//
// Reasons for reissuing a certificate:
// - no certificate yet
// - certificate has the wrong format
// - certificate is invalid or expired
// - certificate has no SAN extra extension
// - certificate SAN and IP does not match pod SAN and IP
func shouldIssueNewCertificate(
	es v1alpha1.Elasticsearch,
	secret corev1.Secret,
	pod corev1.Pod,
	privateKey *rsa.PrivateKey,
	ca *certificates.CA,
	certReconcileBefore time.Duration,
) bool {
	certCommonName := buildCertificateCommonName(pod, es.Name, es.Namespace)

	generalNames, err := buildGeneralNames(es, pod)
	if err != nil {
		log.Error(err, "Cannot create GeneralNames for the TLS certificate",
			"namespace", pod.Namespace, "pod_name", pod.Name)
		return true
	}

	cert := extractTransportCert(secret, pod, certCommonName)
	if cert == nil {
		return true
	}

	publicKey, publicKeyOk := cert.PublicKey.(*rsa.PublicKey)
	if !publicKeyOk || publicKey.N.Cmp(privateKey.PublicKey.N) != 0 || publicKey.E != privateKey.PublicKey.E {
		log.Info(
			"Certificate belongs do a different public key, should issue new",
			"namespace", pod.Namespace,
			"subject", cert.Subject,
			"issuer", cert.Issuer,
			"current_ca_subject", ca.Cert.Subject,
			"pod_name", pod.Name,
		)
		return true
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	verifyOpts := x509.VerifyOptions{
		DNSName:       certCommonName,
		Roots:         pool,
		Intermediates: pool,
	}
	if _, err := cert.Verify(verifyOpts); err != nil {
		log.Info(
			fmt.Sprintf("Certificate was not valid, should issue new: %s", err),
			"namespace", pod.Namespace,
			"subject", cert.Subject,
			"issuer", cert.Issuer,
			"current_ca_subject", ca.Cert.Subject,
			"pod", pod.Name,
		)
		return true
	}

	if time.Now().After(cert.NotAfter.Add(-certReconcileBefore)) {
		log.Info("Certificate soon to expire, should issue new",
			"namespace", pod.Namespace, "pod", pod.Name)
		return true
	}

	// compare actual vs. expected SANs
	expected, err := certificates.MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		log.Error(err, "Cannot marshal subject alternative names, will issue new certificate",
			"namespace", pod.Namespace, "pod_name", pod.Name)
		return true
	}
	extraExtensionFound := false
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(certificates.SubjectAlternativeNamesObjectIdentifier) {
			continue
		}
		extraExtensionFound = true
		if !reflect.DeepEqual(ext.Value, expected) {
			log.Info("Certificate SANs do not match expected one, should issue new",
				"namespace", pod.Namespace, "pod_name", pod.Name)
			return true
		}
	}
	if !extraExtensionFound {
		log.Error(errors.New("no SAN extra extension"),
			"SAN extra extension not found, should issue new certificate",
			"namespace", pod.Namespace, "pod_name", pod.Name)
		return true
	}

	return false
}

// extractTransportCert extracts the transport certificate for the pod with the commonName from the Secret
func extractTransportCert(secret corev1.Secret, pod corev1.Pod, commonName string) *x509.Certificate {
	certData, ok := secret.Data[PodCertFileName(pod.Name)]
	if !ok {
		log.Info("No tls certificate found in secret",
			"namespace", pod.Namespace, "pod_name", pod.Name)
		return nil
	}

	certs, err := certificates.ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found",
			"namespace", pod.Namespace, "pod_name", pod.Name)
		return nil
	}

	// look for the certificate based on the CommonName
	names := make([]string, 0, len(certs))
	for _, c := range certs {
		if c.Subject.CommonName == commonName {
			return c
		}
		names = append(names, c.Subject.CommonName)
	}

	log.Info(
		"Did not find a certificate with the expected common name",
		"namespace", pod.Namespace,
		"pod_name", pod.Name,
		"expected_name", commonName,
		"actual_name", names,
	)

	return nil
}
