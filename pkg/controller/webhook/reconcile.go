// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
)

// Params are params to create and manage the webhook resources (Cert secret and ValidatingWebhookConfiguration)
type Params struct {
	Namespace                string
	SecretName               string
	WebhookConfigurationName string

	// Certificate options
	Rotation certificates.RotationParams
}

// ReconcileResources reconciles the certificates used by the webhook client and the webhook server.
// It also returns the duration after which a certificate rotation should be scheduled.
func (w *Params) ReconcileResources(clientset kubernetes.Interface) error {
	// retrieve current webhook server cert secret
	webhookServerSecret, err := clientset.CoreV1().Secrets(w.Namespace).Get(w.SecretName, metav1.GetOptions{})
	if err != nil {
		// 404 is still considered as an error, webhook secret is expected to be created before the operator is started
		return err
	}

	// retrieve the current webhook configuration
	webhookConfiguration, err := clientset.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(w.WebhookConfigurationName, metav1.GetOptions{})
	if err != nil {
		// 404 is also considered as an error, webhook configuration is expected to be created before the operator is started
		return err
	}

	// check if we need to renew the certificates used in the resources
	if w.shouldRenewCertificates(webhookServerSecret, webhookConfiguration) {
		log.Info(
			"Creating new webhook certificates",
			"webhook", webhookConfiguration.Name,
			"secret_namespace", webhookServerSecret.Namespace,
			"secret_name", webhookServerSecret.Name,
		)
		newCertificates, err := w.newCertificates()
		if err != nil {
			return err
		}
		// update the webhook configuration
		for i := range webhookConfiguration.Webhooks {
			webhookConfiguration.Webhooks[i].ClientConfig.CABundle = newCertificates.caCert
		}
		if _, err := clientset.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Update(webhookConfiguration); err != nil {
			return err
		}

		// update server secret
		webhookServerSecret.Data = map[string][]byte{
			certificates.CertFileName: newCertificates.serverCert,
			certificates.KeyFileName:  newCertificates.serverKey,
		}
		if _, err := clientset.CoreV1().Secrets(w.Namespace).Update(webhookServerSecret); err != nil {
			return err
		}
	}

	return nil
}
