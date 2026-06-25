// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the APM Server validating webhook.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, exposedNodeLabels commonnodelabels.NodeLabels, managedNamespaces []string) {
	inner := &webhookValidator{exposedNodeLabels: exposedNodeLabels}
	v := commonwebhook.NewResourceValidator[*apmv1.ApmServer](checker, managedNamespaces, inner)
	wh := admission.WithValidator[*apmv1.ApmServer](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(apmv1.WebhookPath, wh)
	ulog.Log.Info("Registering APM Server validating webhook", "path", apmv1.WebhookPath)
}

type webhookValidator struct {
	exposedNodeLabels commonnodelabels.NodeLabels
}

func (v *webhookValidator) ValidateCreate(_ context.Context, obj *apmv1.ApmServer) (admission.Warnings, error) {
	return validateApmServer(obj, nil, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateUpdate(_ context.Context, oldObj, newObj *apmv1.ApmServer) (admission.Warnings, error) {
	return validateApmServer(newObj, oldObj, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *apmv1.ApmServer) (admission.Warnings, error) {
	return nil, nil
}

// validateApmServer runs the APM Server validation together with the operator's exposed-node-labels
// policy check. Both the validating webhook and the reconciler call it so the same rules are
// enforced through a single function, regardless of whether the webhook is enabled.
func validateApmServer(as *apmv1.ApmServer, old *apmv1.ApmServer, exposedNodeLabels commonnodelabels.NodeLabels) (admission.Warnings, error) {
	warnings, err := apmv1.Validate(as, old)
	if err != nil {
		return warnings, err
	}
	if errs := commonnodelabels.ValidateAnnotation(as.Annotations, exposedNodeLabels); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: apmv1.GroupVersion.Group, Kind: apmv1.Kind},
			as.Name, errs)
	}
	return warnings, nil
}
