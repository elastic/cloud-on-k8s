// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"time"

	"k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const WebhookServiceName = "elastic-webhook-server"

// WebhookCertificates holds the artifacts used by the webhook server and the webhook configuration.
type WebhookCertificates struct {
	caCert []byte

	serverKey  []byte
	serverCert []byte
}

func (w *Params) shouldRenewCertificates(serverCertificates *corev1.Secret, webhookConfiguration *v1beta1.ValidatingWebhookConfiguration) bool {
	// Read the current certificate used by the server
	serverCA := certificates.BuildCAFromSecret(*serverCertificates)
	if serverCA == nil {
		return true
	}
	if !certificates.CanReuseCA(serverCA, w.Rotation.RotateBefore) {
		return true
	}
	// Read the certificate in the webhook configuration
	for _, webhook := range webhookConfiguration.Webhooks {
		caBytes := webhook.ClientConfig.CABundle
		if len(caBytes) == 0 {
			return true
		}
		// Parse the certificates
		certs, err := certificates.ParsePEMCerts(caBytes)
		if err != nil {
			log.Error(err, "Cannot parse PEM cert from webhook configuration, will create a new one", "webhook_name", webhookConfiguration.Name)
			return true
		}
		if len(certs) == 0 {
			return true
		}
		for _, cert := range certs {
			if !certificates.CertIsValid(*cert, w.Rotation.RotateBefore) {
				return true
			}
		}
	}
	return false
}

// newCertificates creates a new certificate authority and uses it to sign a new key/cert pair for the webhook server.
// The certificate of the CA is used in the webhook configuration so it can be used by the API server to verify the
// certificate of the webhook server.
// The private key is not retained or persisted, all the artifacts are regenerated and updated if needed when the
// certificate is about to expire or is missing.
func (w *Params) newCertificates() (WebhookCertificates, error) {
	webhookCertificates := WebhookCertificates{}

	// Create a new CA
	ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         "elastic-webhook-ca",
			OrganizationalUnit: []string{"elastic-webhook"},
		},
		ExpireIn: &w.Rotation.Validity,
	})
	if err != nil {
		return webhookCertificates, err
	}
	webhookCertificates.caCert = certificates.EncodePEMCert(ca.Cert.Raw)

	// Create a new certificate for the webhook server
	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return webhookCertificates, err
	}
	webhookCertificates.serverKey = certificates.EncodePEMPrivateKey(*privateKey)

	csr, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, privateKey)
	if err != nil {
		return webhookCertificates, err
	}
	parsedCSR, err := x509.ParseCertificateRequest(csr)
	if err != nil {
		return webhookCertificates, err
	}

	certificateTemplate := certificates.ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         "elastic-webhook",
			OrganizationalUnit: []string{"elastic-webhook"},
		},

		DNSNames: k8s.GetServiceDNSName(corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: w.Namespace,
				Name:      WebhookServiceName,
			},
		}),

		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(w.Rotation.Validity),

		PublicKeyAlgorithm: parsedCSR.PublicKeyAlgorithm,
		PublicKey:          parsedCSR.PublicKey,

		Signature:          parsedCSR.Signature,
		SignatureAlgorithm: parsedCSR.SignatureAlgorithm,

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})

	cert, err := ca.CreateCertificate(certificateTemplate)
	if err != nil {
		return webhookCertificates, err
	}
	webhookCertificates.serverCert = certificates.EncodePEMCert(cert)
	return webhookCertificates, nil
}
