// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"context"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/webhook/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/pkg/webhook/license"
	admission "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

const (
	admissionServerName = "elastic-admission-server"
	svcName             = "elastic-webhook-service"
	controlPlane        = "control-plane"

	serverPort int32 = 9443
)

// RegisterValidations registers validating webhooks and a new webhook server with the given manager.
func RegisterValidations(mgr manager.Manager, params Parameters) error {
	esWh, err := builder.NewWebhookBuilder().
		Name("validation.elasticsearch.elastic.co").
		Validating().
		FailurePolicy(admission.Ignore).
		ForType(&v1alpha1.Elasticsearch{}).
		Handlers(&elasticsearch.ValidationHandler{}).
		WithManager(mgr).
		Build()
	if err != nil {
		return err
	}

	licWh, err := builder.NewWebhookBuilder().
		Name("validation.license.elastic.co").
		Validating().
		FailurePolicy(admission.Ignore).
		ForType(&corev1.Secret{}).
		Handlers(&license.ValidationHandler{}).
		WithManager(mgr).
		Build()
	if err != nil {
		return err
	}

	disabled := !params.AutoInstall
	if params.AutoInstall {
		// nasty side effect in register function
		webhookSvc := corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:      params.Bootstrap.Service.Name,
				Namespace: params.Bootstrap.Service.Namespace,
			},
		}
		// best effort deletion attempt to handle incompatible services from previous versions
		_ = mgr.GetClient().Delete(context.Background(), &webhookSvc)
	}
	svr, err := webhook.NewServer(admissionServerName, mgr, webhook.ServerOptions{
		Port:                          serverPort,
		CertDir:                       "/tmp/cert",
		DisableWebhookConfigInstaller: &disabled,
		BootstrapOptions:              &params.Bootstrap,
	})
	if err != nil {
		return err
	}

	return svr.Register(esWh, licWh)
}
