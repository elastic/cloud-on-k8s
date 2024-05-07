// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

// +kubebuilder:webhook:path=/validate-autoscaling-k8s-elastic-co-v1alpha1-elasticsearchautoscaler,mutating=false,failurePolicy=ignore,groups=autoscaling.k8s.elastic.co,resources=elasticsearchautoscalers,verbs=create;update,versions=v1alpha1,name=elastic-esa-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

const (
	webhookPath = "/validate-autoscaling-k8s-elastic-co-v1alpha1-elasticsearchautoscaler"
)

var esalog = ulog.Log.WithName("esa-validation")

// RegisterWebhook will register the Elasticsearch validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, licenseChecker license.Checker, managedNamespaces []string) {
	wh := &validatingWebhook{
		client:               mgr.GetClient(),
		decoder:              admission.NewDecoder(mgr.GetScheme()),
		validateStorageClass: validateStorageClass,
		licenseChecker:       licenseChecker,
		managedNamespaces:    set.Make(managedNamespaces...),
	}
	esalog.Info("Registering ElasticsearchAutoscaler validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type validatingWebhook struct {
	client               k8s.Client
	decoder              admission.Decoder
	validateStorageClass bool
	licenseChecker       license.Checker
	managedNamespaces    set.StringSet
}

func (wh *validatingWebhook) validate(ctx context.Context, esa v1alpha1.ElasticsearchAutoscaler) error {
	esalog.V(1).Info("validate autoscaler", "name", esa.Name, "namespace", esa.Namespace)
	validationError, runtimeErr := ValidateElasticsearchAutoscaler(
		ctx,
		wh.client,
		esa,
		wh.licenseChecker,
	)
	if runtimeErr != nil {
		esalog.Error(
			runtimeErr,
			"Runtime error while validating ElasticsearchAutoscaler manifest",
			"namespace", esa.Namespace,
			"esa_name", esa.Name,
		)
		// ignore non validation errors in the admission controller.
	}
	return validationError
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

// Handle is called when any request is sent to the webhook, satisfying the admission.Handler interface.
func (wh *validatingWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	esa := &v1alpha1.ElasticsearchAutoscaler{}
	err := wh.decoder.DecodeRaw(req.Object, esa)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// If this Elasticsearch instance is not within the set of managed namespaces
	// for this operator ignore this request.
	if wh.managedNamespaces.Count() > 0 && !wh.managedNamespaces.Has(esa.Namespace) {
		esalog.V(1).Info("Skip Elasticsearch resource validation", "name", esa.Name, "namespace", esa.Namespace)
		return admission.Allowed("")
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		err = wh.validate(ctx, *esa)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	return admission.Allowed("")
}
