// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

var log = ulog.Log.WithName("stackconfigpolicy-validation")

// RegisterWebhook registers the StackConfigPolicy validating webhook with the manager.
// operatorNamespace is forwarded to Validate so the namespace-scoping rule is enforced at
// admission time.
func RegisterWebhook(mgr ctrl.Manager, licenseChecker license.Checker, managedNamespaces []string, operatorNamespace string) {
	v := commonwebhook.NewResourceValidator[*policyv1alpha1.StackConfigPolicy](licenseChecker, managedNamespaces, &webhookValidator{operatorNamespace: operatorNamespace})
	wh := admission.WithValidator[*policyv1alpha1.StackConfigPolicy](mgr.GetScheme(), v)
	log.Info("Registering StackConfigPolicy validating webhook", "path", policyv1alpha1.WebhookPath)
	mgr.GetWebhookServer().Register(policyv1alpha1.WebhookPath, wh)
}

type webhookValidator struct {
	operatorNamespace string
}

func (v *webhookValidator) ValidateCreate(_ context.Context, p *policyv1alpha1.StackConfigPolicy) (admission.Warnings, error) {
	return Validate(p, v.operatorNamespace)
}

func (v *webhookValidator) ValidateUpdate(_ context.Context, _, newObj *policyv1alpha1.StackConfigPolicy) (admission.Warnings, error) {
	return Validate(newObj, v.operatorNamespace)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *policyv1alpha1.StackConfigPolicy) (admission.Warnings, error) {
	return nil, nil
}
