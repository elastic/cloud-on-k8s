// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"context"

	v1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// webhooks returns the list of webhook in the configuration
	webhooks() []webhook
	// update ca bundle with the provided CA in all the Webhooks
	updateCABundle(caCert []byte) error
}

func (w *Params) NewAdmissionControllerInterface(ctx context.Context, clientset kubernetes.Interface) (AdmissionControllerInterface, error) {
	// Detect if V1 is available
	_, err := clientset.Discovery().ServerResourcesForGroupVersion(v1.SchemeGroupVersion.String())
	if errors.IsNotFound(err) { // Presumably a K8S cluster older than 1.16
		log.V(1).Info("admissionregistration.k8s.io/v1 is not available, using v1beta1 for webhook configuration")
		webhookConfiguration, err := clientset.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(ctx, w.Name, metav1.GetOptions{})
		if err != nil {
			// 404 is also considered as an error, webhook configuration is expected to be created before the operator is started
			return nil, err
		}
		return &v1beta1webhookHandler{ctx: ctx, clientset: clientset, webhookConfiguration: webhookConfiguration}, nil
	}
	if err != nil {
		return nil, err
	}
	log.V(1).Info(" using admissionregistration.k8s.io/v1 for webhook configuration")
	webhookConfiguration, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, w.Name, metav1.GetOptions{})
	if err != nil {
		// 404 is also considered as an error, webhook configuration is expected to be created before the operator is started
		return nil, err
	}
	return &v1webhookHandler{ctx: ctx, clientset: clientset, webhookConfiguration: webhookConfiguration}, nil
}

// - admissionregistration.k8s.io/v1 implementation

var _ AdmissionControllerInterface = &v1webhookHandler{}

type v1webhookHandler struct {
	clientset            kubernetes.Interface
	ctx                  context.Context
	webhookConfiguration *v1.ValidatingWebhookConfiguration
}

func (_ *v1webhookHandler) getType() client.Object {
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

func (v1w *v1webhookHandler) updateCABundle(caCert []byte) error {
	for i := range v1w.webhookConfiguration.Webhooks {
		v1w.webhookConfiguration.Webhooks[i].ClientConfig.CABundle = caCert
	}
	_, err := v1w.clientset.
		AdmissionregistrationV1().
		ValidatingWebhookConfigurations().
		Update(v1w.ctx, v1w.webhookConfiguration, metav1.UpdateOptions{})
	return err
}

// - admissionregistration.k8s.io/v1beta1 implementation

var _ AdmissionControllerInterface = &v1beta1webhookHandler{}

type v1beta1webhookHandler struct {
	clientset            kubernetes.Interface
	ctx                  context.Context
	webhookConfiguration *v1beta1.ValidatingWebhookConfiguration
}

func (_ *v1beta1webhookHandler) getType() client.Object {
	return &v1beta1.ValidatingWebhookConfiguration{}
}

func (v1beta1w *v1beta1webhookHandler) webhooks() []webhook {
	webhooks := make([]webhook, 0, len(v1beta1w.webhookConfiguration.Webhooks))
	for _, wh := range v1beta1w.webhookConfiguration.Webhooks {
		webhook := webhook{
			webhookConfigurationName: v1beta1w.webhookConfiguration.Name,
			caBundle:                 wh.ClientConfig.CABundle,
		}
		webhooks = append(webhooks, webhook)
	}
	return webhooks
}

func (v1beta1w *v1beta1webhookHandler) services() Services {
	services := make(map[types.NamespacedName]struct{})
	for _, wh := range v1beta1w.webhookConfiguration.Webhooks {
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

func (v1beta1w *v1beta1webhookHandler) updateCABundle(caCert []byte) error {
	for i := range v1beta1w.webhookConfiguration.Webhooks {
		v1beta1w.webhookConfiguration.Webhooks[i].ClientConfig.CABundle = caCert
	}
	_, err := v1beta1w.clientset.
		AdmissionregistrationV1beta1().
		ValidatingWebhookConfigurations().
		Update(v1beta1w.ctx, v1beta1w.webhookConfiguration, metav1.UpdateOptions{})
	return err
}
