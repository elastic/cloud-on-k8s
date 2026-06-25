// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the Beat validating webhook.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, exposedNodeLabels commonnodelabels.NodeLabels, managedNamespaces []string) {
	inner := &webhookValidator{exposedNodeLabels: exposedNodeLabels}
	v := commonwebhook.NewResourceValidator[*beatv1beta1.Beat](checker, managedNamespaces, inner)
	wh := admission.WithValidator[*beatv1beta1.Beat](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(beatv1beta1.WebhookPath, wh)
	ulog.Log.Info("Registering Beat validating webhook", "path", beatv1beta1.WebhookPath)
}

type webhookValidator struct {
	exposedNodeLabels commonnodelabels.NodeLabels
}

func (v *webhookValidator) ValidateCreate(_ context.Context, obj *beatv1beta1.Beat) (admission.Warnings, error) {
	return validateBeat(obj, nil, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateUpdate(_ context.Context, oldObj, newObj *beatv1beta1.Beat) (admission.Warnings, error) {
	return validateBeat(newObj, oldObj, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *beatv1beta1.Beat) (admission.Warnings, error) {
	return nil, nil
}

// validateBeat runs the Beat validation together with the operator's exposed-node-labels policy
// check. Both the validating webhook and the reconciler call it so the same rules are enforced
// through a single function, regardless of whether the webhook is enabled.
func validateBeat(b *beatv1beta1.Beat, old *beatv1beta1.Beat, exposedNodeLabels commonnodelabels.NodeLabels) (admission.Warnings, error) {
	warnings, err := beatv1beta1.Validate(b, old)
	if err != nil {
		return warnings, err
	}
	if errs := commonnodelabels.ValidateAnnotation(b.Annotations, exposedNodeLabels); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: beatv1beta1.GroupVersion.Group, Kind: beatv1beta1.Kind},
			b.Name, errs)
	}
	return warnings, nil
}
