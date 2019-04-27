// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"net/http"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	commonvalidation "github.com/elastic/k8s-operators/operators/pkg/controller/common/validation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/license/validation"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/api/admission/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var log = logf.Log.WithName("license-validation")

// ValidationHandler exposes License validations as an admission.Handler.
type ValidationHandler struct {
	client  client.Client
	decoder types.Decoder
}

var _ inject.Client = &ValidationHandler{}

// TODO explore potential for generalisation here

// Handle processes AdmissionRequests.
func (v *ValidationHandler) Handle(ctx context.Context, r types.Request) types.Response {
	if r.AdmissionRequest.Operation == v1beta1.Delete {
		return admission.ValidationResponse(true, "allowing all deletes")
	}
	license := estype.EnterpriseLicense{}
	log.Info("ValidationHandler handler called",
		"operation", r.AdmissionRequest.Operation,
		"name", r.AdmissionRequest.Name,
		"namespace", r.AdmissionRequest.Namespace,
	)
	err := v.decoder.Decode(r, &license)
	if err != nil {
		log.Error(err, "Failed to decode request")
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	var onServer estype.EnterpriseLicense
	err = v.client.Get(ctx, k8s.ExtractNamespacedName(&license), &onServer)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to retrieve existing cluster")
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	var current *estype.EnterpriseLicense
	if err == nil {
		current = &onServer
	}
	var results []commonvalidation.Result
	validationCtx := &validation.Context{Current: current, Proposed: license}
	for _, v := range validation.Validations {
		results = append(results, v(*validationCtx))
	}
	return aggregate(results)
}

func aggregate(results []commonvalidation.Result) types.Response {
	response := commonvalidation.Result{Allowed: true}
	for _, r := range results {
		if !r.Allowed {
			response.Allowed = false
			if r.Error != nil {
				log.Error(r.Error, r.Reason)
			}
			if response.Reason == "" {
				response.Reason = r.Reason
				continue
			}
			response.Reason = response.Reason + ". " + r.Reason
		}
	}
	log.V(1).Info("Admission validation response", "allowed", response.Allowed, "reason", response.Reason)
	return admission.ValidationResponse(response.Allowed, response.Reason)
}

var _ admission.Handler = &ValidationHandler{}

func (v *ValidationHandler) InjectDecoder(d types.Decoder) error {
	v.decoder = d
	return nil
}

var _ inject.Decoder = &ValidationHandler{}

func (v *ValidationHandler) InjectClient(c client.Client) error {
	v.client = c
	return nil
}
