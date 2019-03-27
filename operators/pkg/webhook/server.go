// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/webhook/elasticsearch"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

const (
	admissionServerName = "elastic-admission-server"
	svcName             = "elastic-global-operator"
	controlPlane        = "control-plane"
)

// BootstrapOptionsParams are params to create webhook BootstrapOptions.
type BootstrapOptionsParams struct {
	Namespace       string
	SecretName      string
	ServiceSelector string
}

// NewBootstrapOptions are options for the webhook bootstrap process.
func NewBootstrapOptions(params BootstrapOptionsParams) webhook.BootstrapOptions {
	var secret *types.NamespacedName
	if params.SecretName != "" {
		secret = &types.NamespacedName{
			Namespace: params.Namespace,
			Name:      params.SecretName,
		}
	}
	var svc *webhook.Service
	if params.ServiceSelector != "" {
		svc = &webhook.Service{
			Namespace: params.Namespace,
			Name:      svcName,
			Selectors: map[string]string{
				controlPlane: params.ServiceSelector,
			},
		}
	}
	return webhook.BootstrapOptions{
		Secret:  secret,
		Service: svc,
	}
}

// RegisterValidations registers validating webhooks and a new webhook server with the given manager.
func RegisterValidations(mgr manager.Manager, params Parameters) error {
	wh, err := builder.NewWebhookBuilder().
		Validating().
		ForType(&v1alpha1.Elasticsearch{}).
		Handlers(&elasticsearch.Validation{}).
		WithManager(mgr).
		Build()
	if err != nil {
		return err
	}

	disabled := !params.AutoInstall
	svr, err := webhook.NewServer(admissionServerName, mgr, webhook.ServerOptions{
		CertDir:                       "/tmp/cert",
		DisableWebhookConfigInstaller: &disabled,
		BootstrapOptions:              &params.Bootstrap,
	})
	if err != nil {
		return err
	}
	return svr.Register(wh)
}
