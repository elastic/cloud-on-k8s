// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the Elastic Maps Server validating webhook.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, exposedNodeLabels commonnodelabels.NodeLabels, managedNamespaces []string) {
	inner := &webhookValidator{exposedNodeLabels: exposedNodeLabels}
	v := commonwebhook.NewResourceValidator[*emsv1alpha1.ElasticMapsServer](checker, managedNamespaces, inner)
	wh := admission.WithValidator[*emsv1alpha1.ElasticMapsServer](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(emsv1alpha1.WebhookPath, wh)
	ulog.Log.Info("Registering Elastic Maps Server validating webhook", "path", emsv1alpha1.WebhookPath)
}

type webhookValidator struct {
	exposedNodeLabels commonnodelabels.NodeLabels
}

func (v *webhookValidator) ValidateCreate(_ context.Context, obj *emsv1alpha1.ElasticMapsServer) (admission.Warnings, error) {
	return validateMapsServer(obj, nil, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateUpdate(_ context.Context, oldObj, newObj *emsv1alpha1.ElasticMapsServer) (admission.Warnings, error) {
	return validateMapsServer(newObj, oldObj, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *emsv1alpha1.ElasticMapsServer) (admission.Warnings, error) {
	return nil, nil
}

// validateMapsServer runs the Elastic Maps Server validation together with the operator's
// exposed-node-labels policy check. Both the validating webhook and the reconciler call it so the
// same rules are enforced through a single function, regardless of whether the webhook is enabled.
func validateMapsServer(m *emsv1alpha1.ElasticMapsServer, old *emsv1alpha1.ElasticMapsServer, exposedNodeLabels commonnodelabels.NodeLabels) (admission.Warnings, error) {
	warnings, err := emsv1alpha1.Validate(m, old)
	if err != nil {
		return warnings, err
	}
	if errs := commonnodelabels.ValidateAnnotation(m.Annotations, exposedNodeLabels); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: emsv1alpha1.GroupVersion.Group, Kind: emsv1alpha1.Kind},
			m.Name, errs)
	}
	return warnings, nil
}
