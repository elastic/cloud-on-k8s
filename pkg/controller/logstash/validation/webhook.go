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

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// +kubebuilder:webhook:path=/validate-logstash-k8s-elastic-co-v1alpha1-logstash,mutating=false,failurePolicy=ignore,groups=logstash.k8s.elastic.co,resources=logstashes,verbs=create;update,versions=v1alpha1,name=elastic-logstash-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-logstash-k8s-elastic-co-v1alpha1-logstash"
)

var lslog = ulog.Log.WithName("ls-validation")

// RegisterWebhook registers the Logstash validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, managedNamespaces []string) {
	inner := &validator{
		client:               mgr.GetClient(),
		validateStorageClass: validateStorageClass,
	}
	// Logstash has no license-dependent validation, so we pass nil here.
	v := commonwebhook.NewResourceValidator[*lsv1alpha1.Logstash](nil, managedNamespaces, inner)
	lslog.Info("Registering Logstash validating webhook", "path", webhookPath)
	wh := admission.WithValidator[*lsv1alpha1.Logstash](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(webhookPath, wh)
}

type validator struct {
	client               k8s.Client
	validateStorageClass bool
}

func (v *validator) ValidateCreate(_ context.Context, ls *lsv1alpha1.Logstash) (admission.Warnings, error) {
	lslog.V(1).Info("validate create", "name", ls.Name)
	warnings, validationErr := ValidateLogstash(ls)
	// Reserved-VCT-label validation runs on create only (no current CR to compare against).
	// On update, checkPVCReservedLabels handles diff-aware checks with grandfathering for
	// already-present (key, value) pairs. This check is intentionally NOT in validations()
	// because ValidateLogstash is also invoked by the reconciler on every reconcile pass,
	// which would otherwise reject any CR carrying grandfathered reserved keys.
	if errs := checkPVCReservedLabelsOnCreate(ls); len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "logstash.k8s.elastic.co", Kind: lsv1alpha1.Kind},
			ls.Name, errs)
	}
	return warnings, validationErr
}

func (v *validator) ValidateUpdate(ctx context.Context, oldObj, newObj *lsv1alpha1.Logstash) (admission.Warnings, error) {
	lslog.V(1).Info("validate update", "name", newObj.Name)

	// Match Elasticsearch: run full validation on the new object first so warnings are collected before
	// update-only checks; when update-only checks fail, return those errors but keep prior warnings.
	warnings, valErr := ValidateLogstash(newObj)

	var errs field.ErrorList
	for _, val := range updateValidations(ctx, v.client, v.validateStorageClass) {
		if err := val(oldObj, newObj); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "logstash.k8s.elastic.co", Kind: lsv1alpha1.Kind},
			newObj.Name, errs)
	}
	return warnings, valErr
}

func (v *validator) ValidateDelete(_ context.Context, _ *lsv1alpha1.Logstash) (admission.Warnings, error) {
	return nil, nil
}

// ValidateLogstash validates a Logstash instance against a set of validation funcs.
// Returns any admission warnings plus an error if validation fails.
func ValidateLogstash(ls *lsv1alpha1.Logstash) (admission.Warnings, error) {
	var warnings admission.Warnings
	// CheckDeprecatedStackVersion's second return is field.ErrorList; it is always nil
	// (parse/unsupported-version failures are handled by validations(), not here).
	if w, _ := commonv1.CheckDeprecatedStackVersion(ls.Spec.Version); w != "" {
		warnings = admission.Warnings{w}
	}
	if resourcesWarning := commonv1.PodTemplateResourcesOverrideWarning(
		"spec.resources",
		"spec.podTemplate",
		lsv1alpha1.LogstashContainerName,
		ls.Spec.Resources,
		ls.Spec.PodTemplate,
	); resourcesWarning != "" {
		warnings = append(warnings, resourcesWarning)
	}
	errs := check(ls, validations())
	if len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "logstash.k8s.elastic.co", Kind: lsv1alpha1.Kind},
			ls.Name,
			errs,
		)
	}
	return warnings, nil
}
