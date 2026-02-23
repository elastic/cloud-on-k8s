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

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// +kubebuilder:webhook:path=/validate-autoops-k8s-elastic-co-v1alpha1-autoopsagentpolicies,mutating=false,failurePolicy=ignore,groups=autoops.k8s.elastic.co,resources=autoopsagentpolicies,verbs=create;update,versions=v1alpha1,name=elastic-autoops-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-autoops-k8s-elastic-co-v1alpha1-autoopsagentpolicies"
)

var autoopslog = ulog.Log.WithName("autoops-validation")

// RegisterWebhook registers the AutoOpsAgentPolicy validating webhook with the manager.
func RegisterWebhook(mgr ctrl.Manager, licenseChecker license.Checker, managedNamespaces []string) {
	wh := &validatingWebhook{
		decoder:           admission.NewDecoder(mgr.GetScheme()),
		licenseChecker:    licenseChecker,
		managedNamespaces: set.Make(managedNamespaces...),
	}
	autoopslog.Info("Registering AutoOpsAgentPolicy validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type validatingWebhook struct {
	decoder           admission.Decoder
	licenseChecker    license.Checker
	managedNamespaces set.StringSet
}

// Handle satisfies the admission.Handler interface.
func (wh *validatingWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	policy := &autoopsv1alpha1.AutoOpsAgentPolicy{}
	err := wh.decoder.DecodeRaw(req.Object, policy)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if wh.managedNamespaces.Count() > 0 && !wh.managedNamespaces.Has(policy.Namespace) {
		autoopslog.V(1).Info("Skip AutoOpsAgentPolicy resource validation", "name", policy.Name, "namespace", policy.Namespace)
		return admission.Allowed("")
	}

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		if err := Validate(ctx, policy, wh.licenseChecker); err != nil {
			return admission.Denied(err.Error())
		}
	}

	return admission.Allowed("")
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
