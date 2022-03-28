// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
)

const (
	// UpdateAnnotation is the name of the annotation applied to pods to force kubelet to resync secrets
	UpdateAnnotation = "update.k8s.elastic.co/timestamp"
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
		UpdateOperatorPods(ctx, clientset, w.Namespace)
	}

	return nil
}

func UpdateOperatorPods(ctx context.Context, clientset kubernetes.Interface, operatorNamespace string) {
	// Get all the pods that are related to control-plane label.
	labels := metav1.ListOptions{
		LabelSelector: "control-plane=elastic-operator",
	}
	pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, labels)
	if err != nil {
		return
	}
	for _, pod := range pods.Items {
		UpdateOperatorPod(ctx, pod, clientset)
	}
}

func UpdateOperatorPod(ctx context.Context, pod corev1.Pod, clientset kubernetes.Interface) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[UpdateAnnotation] = time.Now().Format(time.RFC3339Nano)
	if _, err := clientset.CoreV1().Pods(pod.Namespace).Update(ctx, &pod, metav1.UpdateOptions{}); err != nil {
		if errors.IsConflict(err) {
			// Conflicts are expected and will be handled on the next reconcile loop, no need to error out here
			log.V(1).Info("Conflict while updating pod annotation", "namespace", pod.Namespace, "pod_name", pod.Name)
		} else {
			log.Error(err, "failed to update pod annotation",
				"annotation", UpdateAnnotation,
				"namespace", pod.Namespace,
				"pod_name", pod.Name)
		}
	}
}
