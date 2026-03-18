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

// RegisterWebhook will register the Elasticsearch validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, exposedNodeLabels NodeLabels, licenseChecker license.Checker, managedNamespaces []string) {
	inner := &validator{
		client:               mgr.GetClient(),
		validateStorageClass: validateStorageClass,
		exposedNodeLabels:    exposedNodeLabels,
		licenseChecker:       licenseChecker,
	}
	// License checking is handled inside validations() rather than the wrapper,
	// because ValidateElasticsearch is also called from the reconciler where the
	// wrapper is not in the path.
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
	return ValidateElasticsearch(ctx, *es, v.licenseChecker, v.exposedNodeLabels)
}

func (v *validator) ValidateUpdate(ctx context.Context, oldObj, newObj *esv1.Elasticsearch) (admission.Warnings, error) {
	eslog.V(1).Info("validate update", "name", newObj.Name)

	// Ensure we get the warnings from the validation function such that warnings are returned even on denial.
	warnings, validationErr := ValidateElasticsearch(ctx, *newObj, v.licenseChecker, v.exposedNodeLabels)

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

// ValidateElasticsearch validates an Elasticsearch instance against a set of validation funcs.
func ValidateElasticsearch(ctx context.Context, es esv1.Elasticsearch, checker license.Checker, exposedNodeLabels NodeLabels) (admission.Warnings, error) {
	admWarnings, err := runChecks(es, validations(ctx, checker, exposedNodeLabels), nil)
	if err != nil {
		return nil, err
	}
	return runChecks(es, warnings, admWarnings)
}

// runChecks executes the given validations against the Elasticsearch resource.
// It returns any warning message and an error if validation fails.
func runChecks(es esv1.Elasticsearch, validations []validation, admWarnings admission.Warnings) (admission.Warnings, error) {
	warning, errs := check(es, validations)
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: esv1.Kind},
			es.Name,
			errs,
		)
	}
	if warning != "" {
		admWarnings = append(admWarnings, warning)
	}
	return admWarnings, nil
}
