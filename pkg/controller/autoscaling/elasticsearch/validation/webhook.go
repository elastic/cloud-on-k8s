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

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// +kubebuilder:webhook:path=/validate-autoscaling-k8s-elastic-co-v1alpha1-elasticsearchautoscaler,mutating=false,failurePolicy=ignore,groups=autoscaling.k8s.elastic.co,resources=elasticsearchautoscalers,verbs=create;update,versions=v1alpha1,name=elastic-esa-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-autoscaling-k8s-elastic-co-v1alpha1-elasticsearchautoscaler"
)

var esalog = ulog.Log.WithName("esa-validation")

// RegisterWebhook will register the ElasticsearchAutoscaler validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, licenseChecker license.Checker, managedNamespaces []string) {
	inner := &validator{
		client:               mgr.GetClient(),
		validateStorageClass: validateStorageClass,
		licenseChecker:       licenseChecker,
	}
	// License checking is handled inside validations() rather than the wrapper,
	// because ValidateElasticsearchAutoscaler is also called from the reconciler
	// where the wrapper is not in the path.
	v := commonwebhook.NewResourceValidator[*v1alpha1.ElasticsearchAutoscaler](nil, managedNamespaces, inner)
	esalog.Info("Registering ElasticsearchAutoscaler validating webhook", "path", webhookPath)
	wh := admission.WithValidator[*v1alpha1.ElasticsearchAutoscaler](mgr.GetScheme(), v)
	mgr.GetWebhookServer().Register(webhookPath, wh)
}

type validator struct {
	client               k8s.Client
	validateStorageClass bool
	licenseChecker       license.Checker
}

func (v *validator) validate(ctx context.Context, esa *v1alpha1.ElasticsearchAutoscaler) (admission.Warnings, error) {
	esalog.V(1).Info("validate autoscaler", "name", esa.Name, "namespace", esa.Namespace)
	validationError, runtimeErr := ValidateElasticsearchAutoscaler(ctx, v.client, *esa, v.licenseChecker)
	if runtimeErr != nil {
		esalog.Error(
			runtimeErr,
			"Runtime error while validating ElasticsearchAutoscaler manifest",
			"namespace", esa.Namespace,
			"esa_name", esa.Name,
		)
	}
	return nil, validationError
}

func (v *validator) ValidateCreate(ctx context.Context, esa *v1alpha1.ElasticsearchAutoscaler) (admission.Warnings, error) {
	return v.validate(ctx, esa)
}

func (v *validator) ValidateUpdate(ctx context.Context, _, newObj *v1alpha1.ElasticsearchAutoscaler) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *validator) ValidateDelete(_ context.Context, _ *v1alpha1.ElasticsearchAutoscaler) (admission.Warnings, error) {
	return nil, nil
}

// ValidateElasticsearchAutoscaler validates an ElasticsearchAutoscaler instance against a set of validation funcs.
func ValidateElasticsearchAutoscaler(
	ctx context.Context,
	k8sClient k8s.Client,
	esa v1alpha1.ElasticsearchAutoscaler,
	checker license.Checker,
) (validationError error, runtimeError error) {
	validationErrors, runtimeError := check(esa, validations(ctx, k8sClient, checker))
	if len(validationErrors) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "autoscaling.k8s.elastic.co", Kind: v1alpha1.Kind},
			esa.Name,
			validationErrors,
		), runtimeError
	}
	return nil, runtimeError
}

func check(esa v1alpha1.ElasticsearchAutoscaler, validations []validation) (field.ErrorList, error) {
	var validationErrors field.ErrorList
	for _, val := range validations {
		validationError, err := val(esa)
		if validationError != nil {
			validationErrors = append(validationErrors, validationError...)
		}
		if err != nil {
			return validationErrors, err
		}
	}
	return validationErrors, nil
}
