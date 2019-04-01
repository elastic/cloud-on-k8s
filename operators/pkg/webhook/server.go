// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/webhook/elasticsearch"
	admission "k8s.io/api/admissionregistration/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

const (
	admissionServerName = "elastic-admission-server"
	svcName             = "elastic-webhook-service"
	controlPlane        = "control-plane"
)

// RegisterValidations registers validating webhooks and a new webhook server with the given manager.
func RegisterValidations(mgr manager.Manager, params Parameters) error {
	wh, err := builder.NewWebhookBuilder().
		Name("validation.elasticsearch.elastic.co").
		Validating().
		FailurePolicy(admission.Fail).
		ForType(&v1alpha1.Elasticsearch{}).
		Handlers(&elasticsearch.ValidationHandler{}).
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
