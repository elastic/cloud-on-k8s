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
)

// NewBootstrapOptions are options for the webhook bootstrap process.
func NewBootstrapOptions(ns string, secretName, svcSelector string) webhook.BootstrapOptions {
	var secret *types.NamespacedName
	if secretName != "" {
		secret = &types.NamespacedName{
			Namespace: ns,
			Name:      secretName,
		}
	}
	var svc *webhook.Service
	if svcSelector != "" {
		svc = &webhook.Service{
			Namespace: ns,
			Name:      svcName,
			Selectors: map[string]string{
				"control-plane": svcSelector,
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
		BootstrapOptions: &webhook.BootstrapOptions{
			Secret: &types.NamespacedName{
				Namespace: params.Namespace,
				Name:      "webhook-server-secret",
			},
			Service: &webhook.Service{
				Namespace: params.Namespace,
				Name:      "elastic-global-operator",
				Selectors: map[string]string{
					"control-plane": "elastic-global-operator",
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return svr.Register(wh)
}
