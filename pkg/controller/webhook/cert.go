// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// WebhookCertificates holds the artifacts used by the webhook server and the webhook configuration.
type WebhookCertificates struct { //nolint:revive
	caCert []byte

	serverKey  []byte
	serverCert []byte
}

func (w *Params) shouldRenewCertificates(ctx context.Context, serverCertificates *corev1.Secret, webhooks []webhook) bool {
	// Read the current certificate used by the server
	serverCA := certificates.BuildCAFromSecret(ctx, *serverCertificates)
	if serverCA == nil {
		return true
	}
	if !certificates.CanReuseCA(ctx, serverCA, w.Rotation.RotateBefore) {
		return true
	}
	// Read the certificate in the webhook configuration
	for _, webhook := range webhooks {
		caBytes := webhook.caBundle
		if len(caBytes) == 0 {
			return true
		}
		// Parse the certificates
		certs, err := certificates.ParsePEMCerts(caBytes)
		if err != nil {
			ulog.FromContext(ctx).Error(err, "Cannot parse PEM cert from webhook configuration, will create a new one", "webhook_name", webhook.webhookConfigurationName)
			return true
		}
		if len(certs) == 0 {
			return true
		}
		for _, cert := range certs {
			if !certificates.CertIsValid(ctx, *cert, w.Rotation.RotateBefore) {
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
func (w *Params) newCertificates(webhookServices Services) (WebhookCertificates, error) {
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
	webhookCertificates.serverKey, err = certificates.EncodePEMPrivateKey(privateKey)
	if err != nil {
		return webhookCertificates, err
	}

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
		DNSNames:           extractDNSNames(webhookServices),
		NotBefore:          time.Now().Add(-10 * time.Minute),
		NotAfter:           time.Now().Add(w.Rotation.Validity),
		PublicKeyAlgorithm: parsedCSR.PublicKeyAlgorithm,
		PublicKey:          parsedCSR.PublicKey,
		Signature:          parsedCSR.Signature,
		SignatureAlgorithm: parsedCSR.SignatureAlgorithm,
		KeyUsage:           x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})

	cert, err := ca.CreateCertificate(certificateTemplate)
	if err != nil {
		return webhookCertificates, err
	}
	webhookCertificates.serverCert = certificates.EncodePEMCert(cert)
	return webhookCertificates, nil
}

func extractDNSNames(webhookServices Services) []string {
	svcNames := make(map[string]struct{}, len(webhookServices))
	for svcRef := range webhookServices {
		names := k8s.GetServiceDNSName(
			corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: svcRef.Namespace, Name: svcRef.Name}},
		)
		for _, n := range names {
			svcNames[n] = struct{}{}
		}
	}

	dnsNames := make([]string, len(svcNames))
	i := 0

	for n := range svcNames {
		dnsNames[i] = n
		i++
	}

	return dnsNames
}
