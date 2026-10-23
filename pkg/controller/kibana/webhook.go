// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// RegisterWebhook registers the Kibana v1 validating webhook with license-aware validation.
func RegisterWebhook(mgr ctrl.Manager, checker commonlicense.Checker, managedNamespaces []string, matcher *nsmatch.NamespaceMatcher) {
	inner := &webhookValidator{client: mgr.GetClient(), licenseChecker: checker}
	v := commonwebhook.NewResourceValidator[*kbv1.Kibana](checker, managedNamespaces, inner).WithNamespaceMatcher(matcher)
	wh := admission.WithValidator[*kbv1.Kibana](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(kbv1.WebhookPath, wh)
	ulog.Log.Info("Registering Kibana validating webhook", "path", kbv1.WebhookPath)
}

type webhookValidator struct {
	client         k8s.Client
	licenseChecker commonlicense.Checker
}

func (v *webhookValidator) ValidateCreate(ctx context.Context, obj *kbv1.Kibana) (admission.Warnings, error) {
	return v.validate(ctx, obj, nil)
}

func (v *webhookValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *kbv1.Kibana) (admission.Warnings, error) {
	return v.validate(ctx, newObj, oldObj)
}

func (v *webhookValidator) ValidateDelete(_ context.Context, _ *kbv1.Kibana) (admission.Warnings, error) {
	return nil, nil
}

func (v *webhookValidator) validate(ctx context.Context, kb *kbv1.Kibana, old *kbv1.Kibana) (admission.Warnings, error) {
	warnings, err := kbv1.Validate(kb, old)
	if err != nil {
		return warnings, err
	}

	// Metadata-only updates are always admitted: the operator itself writes association annotations on the Kibana,
	// and those writes must keep succeeding after the Enterprise license expires.
	if old != nil && equality.Semantic.DeepEqual(old.Spec, kb.Spec) {
		return warnings, nil
	}

	if errs := validateBackgroundTasksLicense(ctx, v.client, kb, v.licenseChecker); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: kbv1.GroupVersion.Group, Kind: kbv1.Kind},
			kb.Name, errs,
		)
	}
	return warnings, nil
}

// validateBackgroundTasksLicense rejects any create or spec update that keeps spec.backgroundTasks set without a valid
// Enterprise license, whether background tasks are being enabled or were already enabled. Removing
// spec.backgroundTasks is always admitted, and so are metadata-only updates, see validate. The controller is more lenient for an already stored resource: it keeps
// reconciling an existing background tasks pool and only warns, see gateBackgroundTasksOnLicense.
func validateBackgroundTasksLicense(ctx context.Context, c k8s.Client, kb *kbv1.Kibana, checker commonlicense.Checker) field.ErrorList {
	state, err := backgroundTasksStateOf(ctx, c, kb, checker)
	if err != nil {
		// Fail-open: admit the resource on license check or lookup errors to avoid blocking writes during transient
		// outages. The controller fails-closed on the same errors. This asymmetry is deliberate.
		ulog.FromContext(ctx).Error(err, "while checking the Enterprise license during background tasks validation")
		return nil
	}
	switch state {
	case backgroundTasksNewlyEnabledUnlicensed, backgroundTasksAlreadyEnabledUnlicensed:
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("backgroundTasks"), backgroundTasksLicenseRejectMsg),
		}
	default:
		return nil
	}
}
