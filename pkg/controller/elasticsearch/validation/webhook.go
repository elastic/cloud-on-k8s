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

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// +kubebuilder:webhook:path=/validate-elasticsearch-k8s-elastic-co-v1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1,name=elastic-es-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-elasticsearch-k8s-elastic-co-v1-elasticsearch"
)

var eslog = ulog.Log.WithName("es-validation")

// RegisterWebhook registers the Elasticsearch validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, exposedNodeLabels NodeLabels, licenseChecker license.Checker, managedNamespaces []string) {
	inner := &validator{
		client:               mgr.GetClient(),
		validateStorageClass: validateStorageClass,
		exposedNodeLabels:    exposedNodeLabels,
		licenseChecker:       licenseChecker,
	}
	// License checks run inside validations(), so we pass nil here
	// (the reconciler calls ValidateElasticsearch directly).
	v := commonwebhook.NewResourceValidator[*esv1.Elasticsearch](nil, managedNamespaces, inner)
	eslog.Info("Registering Elasticsearch validating webhook", "path", webhookPath)
	wh := admission.WithValidator[*esv1.Elasticsearch](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(webhookPath, wh)
}

type validator struct {
	client               k8s.Client
	validateStorageClass bool
	exposedNodeLabels    NodeLabels
	licenseChecker       license.Checker
}

func (v *validator) ValidateCreate(ctx context.Context, es *esv1.Elasticsearch) (admission.Warnings, error) {
	eslog.V(1).Info("validate create", "name", es.Name)
	return ValidateElasticsearch(ctx, v.client, *es, v.licenseChecker, v.exposedNodeLabels)
}

func (v *validator) ValidateUpdate(ctx context.Context, oldObj, newObj *esv1.Elasticsearch) (admission.Warnings, error) {
	eslog.V(1).Info("validate update", "name", newObj.Name)

	// Ensure we get the warnings from the validation function such that warnings are returned even on denial.
	warnings, validationErr := ValidateElasticsearch(ctx, v.client, *newObj, v.licenseChecker, v.exposedNodeLabels)
	if w := validateRestartTriggerWarnings(ctx, v.client, *oldObj, *newObj); w != "" {
		warnings = append(warnings, w)
	}

	var errs field.ErrorList
	for _, val := range updateValidations(ctx, v.client, v.validateStorageClass) {
		if err := val(*oldObj, *newObj); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: esv1.Kind},
			newObj.Name, errs)
	}
	return warnings, validationErr
}

func (v *validator) ValidateDelete(_ context.Context, _ *esv1.Elasticsearch) (admission.Warnings, error) {
	return nil, nil
}

// ValidateElasticsearch validates an Elasticsearch instance against a set of validation funcs returning warnings and an error if validation fails.
func ValidateElasticsearch(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, checker license.Checker, exposedNodeLabels NodeLabels) (admission.Warnings, error) {
	if err := runChecks(es, validations(ctx, checker, exposedNodeLabels)); err != nil {
		return nil, err
	}
	var admWarnings admission.Warnings
	for _, val := range warnings {
		for _, fieldErr := range val(es) {
			admWarnings = append(admWarnings, fieldErr.Detail)
		}
	}
	fipsWarns, err := fipsWarnings(ctx, c, es)
	if err != nil {
		return nil, err
	}
	for _, fieldErr := range fipsWarns {
		admWarnings = append(admWarnings, fieldErr.Detail)
	}
	if w := validateRestartAllocationDelayWarnings(es); w != "" {
		admWarnings = append(admWarnings, w)
	}
	settingWarns, settingErrs := settingsWarningsAndErrors(es)
	admWarnings = append(admWarnings, settingWarns...)
	if len(settingErrs) > 0 {
		return admWarnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: esv1.Kind},
			es.Name,
			settingErrs,
		)
	}
	return admWarnings, nil
}

// runChecks executes the given validations against the Elasticsearch resource.
// It returns an error if any validation fails.
func runChecks(es esv1.Elasticsearch, validations []validation) error {
	errs := check(es, validations)
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: esv1.Kind},
			es.Name,
			errs,
		)
	}
	return nil
}
