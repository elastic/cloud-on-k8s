// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the Elastic Package Registry validating webhook.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, exposedNodeLabels commonnodelabels.NodeLabels, managedNamespaces []string, matcher *nsmatch.NamespaceMatcher) {
	inner := &webhookValidator{exposedNodeLabels: exposedNodeLabels}
	v := commonwebhook.NewResourceValidator[*eprv1alpha1.PackageRegistry](checker, managedNamespaces, inner).WithNamespaceMatcher(matcher)
	wh := admission.WithValidator[*eprv1alpha1.PackageRegistry](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(eprv1alpha1.WebhookPath, wh)
	ulog.Log.Info("Registering Elastic Package Registry validating webhook", "path", eprv1alpha1.WebhookPath)
}

type webhookValidator struct {
	exposedNodeLabels commonnodelabels.NodeLabels
}

func (v *webhookValidator) ValidateCreate(_ context.Context, obj *eprv1alpha1.PackageRegistry) (admission.Warnings, error) {
	return validatePackageRegistry(obj, nil, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateUpdate(_ context.Context, oldObj, newObj *eprv1alpha1.PackageRegistry) (admission.Warnings, error) {
	return validatePackageRegistry(newObj, oldObj, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *eprv1alpha1.PackageRegistry) (admission.Warnings, error) {
	return nil, nil
}

// validatePackageRegistry runs the Elastic Package Registry validation together with the operator's
// exposed-node-labels policy check. Both the validating webhook and the reconciler call it so the
// same rules are enforced through a single function, regardless of whether the webhook is enabled.
func validatePackageRegistry(epr *eprv1alpha1.PackageRegistry, old *eprv1alpha1.PackageRegistry, exposedNodeLabels commonnodelabels.NodeLabels) (admission.Warnings, error) {
	warnings, err := eprv1alpha1.Validate(epr, old)
	if err != nil {
		return warnings, err
	}
	if errs := commonnodelabels.ValidateAnnotation(epr.Annotations, exposedNodeLabels); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: eprv1alpha1.GroupVersion.Group, Kind: eprv1alpha1.Kind},
			epr.Name, errs)
	}
	return warnings, nil
}
