// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"encoding/json"
	"maps"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
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
	span, ctx := apm.StartSpan(ctx, "reconcile_resources", tracing.SpanTypeApp)
	defer span.End()

	// retrieve current webhook server cert secret
	webhookServerSecret, err := clientset.CoreV1().Secrets(w.Namespace).Get(ctx, w.SecretName, metav1.GetOptions{})
	if err != nil {
		// 404 is still considered as an error, webhook secret is expected to be created before the operator is started
		return err
	}

	if !w.shouldRenewCertificates(ctx, webhookServerSecret, webhookConfiguration.webhooks()) {
		return nil
	}
	return w.renewCertificates(ctx, clientset, webhookServerSecret, webhookConfiguration)
}

// renewCertificates creates a new CA/server cert pair, patches caBundle on the
// ValidatingWebhookConfiguration, and patches the webhook server Secret.
func (w *Params) renewCertificates(
	ctx context.Context,
	clientset kubernetes.Interface,
	webhookServerSecret *corev1.Secret,
	webhookConfiguration AdmissionControllerInterface,
) error {
	ulog.FromContext(ctx).Info(
		"Creating new webhook certificates",
		"webhook", w.Name,
		"secret_namespace", webhookServerSecret.Namespace,
		"secret_name", webhookServerSecret.Name,
	)
	newCertificates, err := w.newCertificates(webhookConfiguration.services())
	if err != nil {
		return err
	}
	if err := webhookConfiguration.updateCABundle(newCertificates.caCert); err != nil {
		return err
	}

	// Metadata+data merge patch: only the TLS material and the watched-resources label are
	// written. Sending the full live labels map keeps Helm-managed labels; a full-object
	// Update would strip them and claim ownership of every other Secret field under SSA.
	labels := maps.Clone(webhookServerSecret.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[commonv1.RestrictWatchedResourcesLabelName] = commonv1.RestrictWatchedResourcesLabelValue

	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"labels":          labels,
			"resourceVersion": webhookServerSecret.ResourceVersion,
		},
		"data": map[string][]byte{
			certificates.CertFileName: newCertificates.serverCert,
			certificates.KeyFileName:  newCertificates.serverKey,
		},
	})
	if err != nil {
		return err
	}
	if _, err := clientset.CoreV1().Secrets(w.Namespace).Patch(
		ctx, webhookServerSecret.Name, types.MergePatchType, patch, metav1.PatchOptions{},
	); err != nil {
		return err
	}
	updateOperatorPods(ctx, clientset, w.Namespace)
	return nil
}

// updateOperatorPods updates a specific annotation on the pods to speed up secret propagation.
func updateOperatorPods(ctx context.Context, clientset kubernetes.Interface, operatorNamespace string) {
	// Get all the pods that are related to control-plane label.
	labels := metav1.ListOptions{
		LabelSelector: "control-plane=elastic-operator",
	}
	pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, labels)
	if err != nil {
		return
	}
	for _, pod := range pods.Items {
		updateOperatorPod(ctx, pod, clientset)
	}
}

// updateOperatorPod updates a specific annotation on a single pod to speed up secret propagation.
func updateOperatorPod(ctx context.Context, pod corev1.Pod, clientset kubernetes.Interface) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the last the version of the Pod
		pod, err := clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		// Annotation-only merge patch: avoid a full-object Update that would claim pod spec
		// ownership under Server-Side Apply (the operator Deployment is Helm-managed).
		patch, err := json.Marshal(map[string]any{
			"metadata": map[string]any{
				"annotations": map[string]string{
					annotation.UpdateAnnotation: time.Now().Format(time.RFC3339Nano),
				},
				"resourceVersion": pod.ResourceVersion,
			},
		})
		if err != nil {
			return err
		}
		_, err = clientset.CoreV1().Pods(pod.Namespace).Patch(
			ctx, pod.Name, types.MergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	})
	if err != nil {
		ulog.FromContext(ctx).Error(err, "failed to update pod annotation",
			"annotation", annotation.UpdateAnnotation,
			"namespace", pod.Namespace,
			"pod_name", pod.Name)
	}
}
