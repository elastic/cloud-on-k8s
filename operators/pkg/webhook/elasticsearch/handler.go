// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"net/http"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"

	"k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var log = logf.Log.WithName("es-webhook")

// Validation are validations of Elasticsearch clusters.
type Validation struct {
	client  client.Client
	decoder types.Decoder
}

type ValidationResult struct {
	Error   error
	Allowed bool
	Reason  string
}

func (v *Validation) Handle(ctx context.Context, r types.Request) types.Response {
	if r.AdmissionRequest.Operation == v1beta1.Delete {
		return admission.ValidationResponse(true, "allowing all deletes")
	}
	esCluster := estype.Elasticsearch{}
	log.Info("Validation handler called",
		"operation", r.AdmissionRequest.Operation,
		"name", r.AdmissionRequest.Name,
		"namespace", r.AdmissionRequest.Namespace,
	)
	err := v.decoder.Decode(r, &esCluster)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	validations := []func(context.Context, estype.Elasticsearch) ValidationResult{
		func(_ context.Context, es estype.Elasticsearch) ValidationResult {
			return HasMaster(es)
		},
		v.canUpgrade,
	}
	var results []ValidationResult
	for _, v := range validations {
		results = append(results, v(ctx, esCluster))
	}
	return aggregate(results)
}

func aggregate(results []ValidationResult) types.Response {
	response := ValidationResult{Allowed: true}
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

var _ admission.Handler = &Validation{}

func (v *Validation) InjectDecoder(d types.Decoder) error {
	v.decoder = d
	return nil
}

var _ inject.Decoder = &Validation{}

func (v *Validation) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

var _ inject.Client = &Validation{}
