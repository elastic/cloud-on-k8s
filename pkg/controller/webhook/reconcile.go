// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"encoding/json"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
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

	// check if we need to renew the certificates used in the resources
	if w.shouldRenewCertificates(ctx, webhookServerSecret, webhookConfiguration.webhooks()) {
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
		// update the webhook configuration
		if err := webhookConfiguration.updateCABundle(newCertificates.caCert); err != nil {
			return err
		}

		// patch only the cert data and the single ECK-owned label, leaving everything else
		// on the Secret (Helm-managed labels, annotations, etc.) untouched. The resourceVersion
		// is embedded so the patch is rejected on conflict. JSON Merge Patch merges nested
		// maps, so unlisted Helm-owned labels are preserved server-side; omitting them from
		// the patch body prevents the API server from attributing their field ownership to
		// the operator's field manager, which would cause Helm SSA Apply conflicts on upgrade.
		patch, err := json.Marshal(map[string]any{
			"data": map[string][]byte{
				certificates.CertFileName: newCertificates.serverCert,
				certificates.KeyFileName:  newCertificates.serverKey,
			},
			"metadata": map[string]any{
				"resourceVersion": webhookServerSecret.ResourceVersion,
				"labels": map[string]string{
					commonv1.RestrictWatchedResourcesLabelName: commonv1.RestrictWatchedResourcesLabelValue,
				},
			},
		})
		if err != nil {
			return err
		}
		if _, err := clientset.CoreV1().Secrets(w.Namespace).Patch(ctx, w.SecretName, types.MergePatchType, patch,
			metav1.PatchOptions{FieldManager: about.FieldOwner}); err != nil {
			return err
		}
		updateOperatorPods(ctx, clientset, w.Namespace)
	}

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
		// Fetch the latest version of the Pod for its resourceVersion.
		pod, err := clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		// Patch only the annotation, leaving all other fields (spec, labels, etc.) that
		// are owned by the Deployment/ReplicaSet controller or by Helm untouched.
		patch, err := json.Marshal(map[string]any{
			"metadata": map[string]any{
				"resourceVersion": pod.ResourceVersion,
				"annotations": map[string]any{
					annotation.UpdateAnnotation: time.Now().Format(time.RFC3339Nano),
				},
			},
		})
		if err != nil {
			return err
		}
		_, err = clientset.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, types.MergePatchType, patch,
			metav1.PatchOptions{FieldManager: about.FieldOwner})
		return err
	})
	if err != nil {
		ulog.FromContext(ctx).Error(err, "failed to update pod annotation",
			"annotation", annotation.UpdateAnnotation,
			"namespace", pod.Namespace,
			"pod_name", pod.Name)
	}
}
