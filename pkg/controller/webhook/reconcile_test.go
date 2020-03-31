// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
)

func TestParams_ReconcileResources(t *testing.T) {

	w := Params{
		Namespace:                "elastic-system",
		SecretName:               "elastic-webhook-server-cert",
		WebhookConfigurationName: "elastic-webhook.k8s.elastic.co",
		Rotation: certificates.RotationParams{
			Validity:     certificates.DefaultCertValidity,
			RotateBefore: certificates.DefaultRotateBefore,
		},
	}

	clientset :=
		fake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "elastic-system",
					Name:      "elastic-webhook-server-cert",
				},
			},
			&v1beta1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "elastic-webhook.k8s.elastic.co",
				},
				Webhooks: []v1beta1.ValidatingWebhook{
					{
						Name:         "elastic-es-validation-v1.k8s.elastic.co",
						ClientConfig: v1beta1.WebhookClientConfig{},
					},
				},
			},
		)

	if err := w.ReconcileResources(clientset); err != nil {
		t.Errorf("Params.ReconcileResources() error = %v", err)
	}

	// Secret and ValidatingWebhookConfiguration must have been filled with the certificates
	// retrieve current webhook server cert secret
	webhookServerSecret, err := clientset.CoreV1().Secrets(w.Namespace).Get(w.SecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(webhookServerSecret.Data))

	// retrieve the current webhook configuration
	webhookConfiguration, err := clientset.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(w.WebhookConfigurationName, metav1.GetOptions{})
	assert.NoError(t, err)
	caBundle := webhookConfiguration.Webhooks[0].ClientConfig.CABundle
	assert.True(t, len(caBundle) > 0)

	// Check that the cert in the secret has been signed by the caBundle
	verifyCertificates(t, caBundle, webhookServerSecret.Data["tls.crt"])

	// Delete the content of the secret, certificates should be recreated
	webhookServerSecret.Data = map[string][]byte{}
	_, err = clientset.CoreV1().Secrets(w.Namespace).Update(webhookServerSecret)
	assert.NoError(t, err)
	webhookServerSecret, err = clientset.CoreV1().Secrets(w.Namespace).Get(w.SecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(webhookServerSecret.Data))

	if err := w.ReconcileResources(clientset); err != nil {
		t.Errorf("Params.ReconcileResources() error = %v", err)
	}

	webhookServerSecret, err = clientset.CoreV1().Secrets(w.Namespace).Get(w.SecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(webhookServerSecret.Data))

	// retrieve the new ca
	webhookConfiguration, err = clientset.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(w.WebhookConfigurationName, metav1.GetOptions{})
	assert.NoError(t, err)
	caBundle = webhookConfiguration.Webhooks[0].ClientConfig.CABundle
	// Check again that the cert in the secret has been signed by the caBundle
	verifyCertificates(t, caBundle, webhookServerSecret.Data["tls.crt"])

}

func verifyCertificates(t *testing.T, rootCert []byte, serverCert []byte) {
	ca := x509.NewCertPool()
	ok := ca.AppendCertsFromPEM(rootCert)
	assert.True(t, ok)
	block, _ := pem.Decode(serverCert)
	assert.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	assert.NoError(t, err)
	opts := x509.VerifyOptions{
		DNSName: "elastic-webhook-server.elastic-system.svc",
		Roots:   ca,
	}
	_, err = cert.Verify(opts)
	assert.NoError(t, err)
}
