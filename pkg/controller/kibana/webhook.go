// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the Kibana validating webhook.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, exposedNodeLabels commonnodelabels.NodeLabels, managedNamespaces []string) {
	inner := &webhookValidator{exposedNodeLabels: exposedNodeLabels}
	v := commonwebhook.NewResourceValidator[*kbv1.Kibana](checker, managedNamespaces, inner)
	wh := admission.WithValidator[*kbv1.Kibana](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(kbv1.WebhookPath, wh)
	ulog.Log.Info("Registering Kibana validating webhook", "path", kbv1.WebhookPath)
}

type webhookValidator struct {
	exposedNodeLabels commonnodelabels.NodeLabels
}

func (v *webhookValidator) ValidateCreate(_ context.Context, obj *kbv1.Kibana) (admission.Warnings, error) {
	return validateKibana(obj, nil, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateUpdate(_ context.Context, oldObj, newObj *kbv1.Kibana) (admission.Warnings, error) {
	return validateKibana(newObj, oldObj, v.exposedNodeLabels)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *kbv1.Kibana) (admission.Warnings, error) {
	return nil, nil
}

// validateKibana runs the Kibana validation together with the operator's exposed-node-labels policy
// check. Both the validating webhook and the reconciler call it so the same rules are enforced
// through a single function, regardless of whether the webhook is enabled.
func validateKibana(k *kbv1.Kibana, old *kbv1.Kibana, exposedNodeLabels commonnodelabels.NodeLabels) (admission.Warnings, error) {
	warnings, err := kbv1.Validate(k, old)
	if err != nil {
		return warnings, err
	}
	if errs := commonnodelabels.ValidateAnnotation(k.Annotations, exposedNodeLabels); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: kbv1.GroupVersion.Group, Kind: kbv1.Kind},
			k.Name, errs)
	}
	return warnings, nil
}
