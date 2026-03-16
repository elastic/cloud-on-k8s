// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// +kubebuilder:webhook:path=/validate-autoops-k8s-elastic-co-v1alpha1-autoopsagentpolicies,mutating=false,failurePolicy=ignore,groups=autoops.k8s.elastic.co,resources=autoopsagentpolicies,verbs=create;update,versions=v1alpha1,name=elastic-autoops-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-autoops-k8s-elastic-co-v1alpha1-autoopsagentpolicies"
)

var autoopslog = ulog.Log.WithName("autoops-validation")

// RegisterWebhook registers the AutoOpsAgentPolicy validating webhook with the manager.
func RegisterWebhook(mgr ctrl.Manager, licenseChecker license.Checker, managedNamespaces []string) {
	autoopsValidator := &validator{
		licenseChecker: licenseChecker,
	}
	// License checking is handled inside validations() rather than the wrapper,
	// because Validate is also called from the reconciler where the wrapper is
	// not in the path.
	v := commonwebhook.NewResourceValidator[*autoopsv1alpha1.AutoOpsAgentPolicy](nil, managedNamespaces, autoopsValidator)
	autoopslog.Info("Registering AutoOpsAgentPolicy validating webhook", "path", webhookPath)
	wh := admission.WithValidator[*autoopsv1alpha1.AutoOpsAgentPolicy](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(webhookPath, wh)
}

type validator struct {
	licenseChecker license.Checker
}

func (v *validator) validate(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy) (admission.Warnings, error) {
	return nil, Validate(ctx, policy, v.licenseChecker)
}

func (v *validator) ValidateCreate(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy) (admission.Warnings, error) {
	return v.validate(ctx, policy)
}

func (v *validator) ValidateUpdate(ctx context.Context, _, newObj *autoopsv1alpha1.AutoOpsAgentPolicy) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *validator) ValidateDelete(_ context.Context, _ *autoopsv1alpha1.AutoOpsAgentPolicy) (admission.Warnings, error) {
	return nil, nil
}

// Validate runs all validations against the policy, including license-aware checks.
// It is used by both the webhook handler and the reconciler.
func Validate(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy, checker license.Checker) error {
	var errs field.ErrorList
	for _, check := range validations(ctx, checker) {
		if err := check(policy); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: autoopsv1alpha1.GroupVersion.Group, Kind: autoopsv1alpha1.Kind},
			policy.Name,
			errs,
		)
	}
	return nil
}
