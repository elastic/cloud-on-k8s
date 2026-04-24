// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the Agent validating webhook with license-aware validation.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, managedNamespaces []string) {
	inner := &webhookValidator{licenseChecker: checker}
	// Pass nil for licenseChecker: the inner validator performs license enforcement itself.
	v := commonwebhook.NewResourceValidator[*agentv1alpha1.Agent](nil, managedNamespaces, inner)
	wh := admission.WithValidator[*agentv1alpha1.Agent](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(agentv1alpha1.WebhookPath, wh)
	ulog.Log.Info("Registering Agent validating webhook", "path", agentv1alpha1.WebhookPath)
}

type webhookValidator struct {
	licenseChecker commonlicense.Checker
}

func (v *webhookValidator) ValidateCreate(ctx context.Context, obj *agentv1alpha1.Agent) (admission.Warnings, error) {
	return v.validate(ctx, obj, nil)
}

func (v *webhookValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *agentv1alpha1.Agent) (admission.Warnings, error) {
	return v.validate(ctx, newObj, oldObj)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *agentv1alpha1.Agent) (admission.Warnings, error) {
	return nil, nil
}

func (v *webhookValidator) validate(ctx context.Context, a *agentv1alpha1.Agent, old *agentv1alpha1.Agent) (admission.Warnings, error) {
	warnings, err := agentv1alpha1.Validate(a, old)
	if err != nil {
		return warnings, err
	}
	if errs := validClientAuthentication(ctx, a, v.licenseChecker); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: agentv1alpha1.GroupVersion.Group, Kind: agentv1alpha1.Kind},
			a.Name, errs,
		)
	}
	return warnings, nil
}

// validClientAuthentication checks that client certificate authentication requires an enterprise license.
func validClientAuthentication(ctx context.Context, a *agentv1alpha1.Agent, checker commonlicense.Checker) field.ErrorList {
	if !a.Spec.HTTP.TLS.Client.Authentication {
		return nil
	}
	enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "while checking enterprise features during client authentication validation")
		return nil
	}
	if !enabled {
		return field.ErrorList{
			field.Forbidden(
				field.NewPath("spec").Child("http", "tls", "client", "authentication"),
				"client certificate authentication requires an enterprise license",
			),
		}
	}
	return nil
}
