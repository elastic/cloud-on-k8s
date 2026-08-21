// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"encoding/json"

	v1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
)

type webhook struct {
	webhookConfigurationName, webhookName string
	caBundle                              []byte
}

type Services map[types.NamespacedName]struct{}

// AdmissionControllerInterface helps to setup webhooks for different versions of the admissionregistration API.
type AdmissionControllerInterface interface {
	getType() client.Object
	// services returns the set of services used by the Webhooks
	services() Services
	// webhooks returns the list of webhooks in the configuration
	webhooks() []webhook
	// updateCABundle updates CABundle with the provided CA in all the Webhooks
	updateCABundle(caCert []byte) error
}

func (w *Params) NewAdmissionControllerInterface(ctx context.Context, clientset kubernetes.Interface) (AdmissionControllerInterface, error) {
	webhookConfiguration, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, w.Name, metav1.GetOptions{})
	if err != nil {
		// 404 is also considered as an error, webhook configuration is expected to be created before the operator is started
		return nil, err
	}
	return &v1webhookHandler{ctx: ctx, clientset: clientset, webhookConfiguration: webhookConfiguration}, nil
}

// - admissionregistration.k8s.io/v1 implementation

var _ AdmissionControllerInterface = (*v1webhookHandler)(nil)

type v1webhookHandler struct {
	clientset            kubernetes.Interface
	ctx                  context.Context
	webhookConfiguration *v1.ValidatingWebhookConfiguration
}

func (*v1webhookHandler) getType() client.Object {
	return &v1.ValidatingWebhookConfiguration{}
}

func (v1w *v1webhookHandler) webhooks() []webhook {
	webhooks := make([]webhook, 0, len(v1w.webhookConfiguration.Webhooks))
	for _, wh := range v1w.webhookConfiguration.Webhooks {
		webhook := webhook{
			webhookConfigurationName: v1w.webhookConfiguration.Name,
			webhookName:              wh.Name,
			caBundle:                 wh.ClientConfig.CABundle,
		}
		webhooks = append(webhooks, webhook)
	}
	return webhooks
}

func (v1w *v1webhookHandler) services() Services {
	services := make(map[types.NamespacedName]struct{})
	for _, wh := range v1w.webhookConfiguration.Webhooks {
		if wh.ClientConfig.Service == nil {
			continue
		}
		services[types.NamespacedName{
			Namespace: wh.ClientConfig.Service.Namespace,
			Name:      wh.ClientConfig.Service.Name,
		}] = struct{}{}
	}
	return services
}

// updateCABundle patches only the clientConfig.caBundle of each webhook entry, matched by name,
// instead of replacing the whole object. The webhooks list is a Kubernetes-native associative list
// keyed by name (+patchStrategy=merge +patchMergeKey=name), so a strategic merge patch here leaves
// every other field of the ValidatingWebhookConfiguration (rules, failurePolicy, selectors, etc.),
// which is managed by the Helm chart, untouched. The resourceVersion read at Get time is embedded
// in the patch to preserve the optimistic-concurrency guarantee.
func (v1w *v1webhookHandler) updateCABundle(caCert []byte) error {
	webhooks := make([]map[string]any, 0, len(v1w.webhookConfiguration.Webhooks))
	for _, wh := range v1w.webhookConfiguration.Webhooks {
		webhooks = append(webhooks, map[string]any{
			"name":         wh.Name,
			"clientConfig": map[string]any{"caBundle": caCert},
		})
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"resourceVersion": v1w.webhookConfiguration.ResourceVersion,
		},
		"webhooks": webhooks,
	})
	if err != nil {
		return err
	}
	_, err = v1w.clientset.
		AdmissionregistrationV1().
		ValidatingWebhookConfigurations().
		Patch(v1w.ctx, v1w.webhookConfiguration.Name, types.StrategicMergePatchType, patch,
			metav1.PatchOptions{FieldManager: about.FieldOwner})
	return err
}
