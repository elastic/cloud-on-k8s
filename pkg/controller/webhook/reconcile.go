// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
)

// Params are params to create and manage the webhook resources (Cert secret and ValidatingWebhookConfiguration)
type Params struct {
	Name       string
	Namespace  string
	SecretName string

	// Certificate options
	Rotation certificates.RotationParams
}

// ReconcileResources reconciles the certificates used by the webhook client and the webhook server.
// It also returns the duration after which a certificate rotation should be scheduled.
func (w *Params) ReconcileResources(ctx context.Context, clientset kubernetes.Interface, webhookConfiguration AdmissionControllerInterface) error {
	// retrieve current webhook server cert secret
	webhookServerSecret, err := clientset.CoreV1().Secrets(w.Namespace).Get(ctx, w.SecretName, metav1.GetOptions{})
	if err != nil {
		// 404 is still considered as an error, webhook secret is expected to be created before the operator is started
		return err
	}

	// check if we need to renew the certificates used in the resources
	if w.shouldRenewCertificates(webhookServerSecret, webhookConfiguration.webhooks()) {
		log.Info(
			"Creating new webhook certificates",
			"webhook", w.Name,
			"secret_namespace", webhookServerSecret.Namespace,
			"secret_name", webhookServerSecret.Name,
		)
		newCertificates, err := w.newCertificates(webhookConfiguration.services())
		if err != nil {
			return err
		}
		// update the webhook configuration
		if err := webhookConfiguration.updateCABundle(newCertificates.caCert); err != nil {
			return err
		}

		// update server secret
		webhookServerSecret.Data = map[string][]byte{
			certificates.CertFileName: newCertificates.serverCert,
			certificates.KeyFileName:  newCertificates.serverKey,
		}
		if _, err := clientset.CoreV1().Secrets(w.Namespace).Update(ctx, webhookServerSecret, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}
