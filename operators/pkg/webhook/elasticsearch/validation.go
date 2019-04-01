// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"errors"
	"net/http"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	"k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	masterRequiredMsg        = "Elasticsearch needs to have at least one master node"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
)

var log = logf.Log.WithName("es-validation")

// Validation is a function from a currently stored Elasticsearch spec and proposed new spec to a ValidationResult.
type Validation func(ctx ValidationContext) ValidationResult

// ValidationResult contains validation results.
type ValidationResult struct {
	Error   error
	Allowed bool
	Reason  string
}

// OK is a successfull validation result.
var OK = ValidationResult{Allowed: true}

// ElasticsearchVersion groups an ES resource and its parsed version.
type ElasticsearchVersion struct {
	Elasticsearch estype.Elasticsearch
	Version       version.Version
}

// ValidationContext is structured input for validation functions.
type ValidationContext struct {
	// Current is the Elasticsearch spec/version currently stored in the api server. Can be nil on new clusters.
	Current *ElasticsearchVersion
	// Proposed is the Elasticsearch spec/version submitted for validation.
	Proposed ElasticsearchVersion
}

// NewValidationContext constructs a new ValidationContext.
func NewValidationContext(current *estype.Elasticsearch, proposed estype.Elasticsearch) (*ValidationContext, error) {
	proposedVersion, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		return nil, pkgerrors.Wrap(err, parseVersionErrMsg)
	}
	ctx := ValidationContext{
		Proposed: ElasticsearchVersion{
			Elasticsearch: proposed,
			Version:       *proposedVersion,
		},
	}
	if current != nil {
		currentVersion, err := version.Parse(current.Spec.Version)
		if err != nil {
			return nil, pkgerrors.Wrap(err, parseStoredVersionErrMsg)
		}
		ctx.Current = &ElasticsearchVersion{
			Elasticsearch: *current,
			Version:       *currentVersion,
		}
	}
	return &ctx, nil
}

func (v ValidationContext) isCreate() bool {
	return v.Current == nil
}

// Validate runs validation logic in contexts where we don't have current and proposed Elasticsearch versions.
func Validate(es estype.Elasticsearch) error {
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return err
	}

	vCtx := ValidationContext{
		Current: nil,
		Proposed: ElasticsearchVersion{
			Elasticsearch: es,
			Version:       *v,
		},
	}
	var errs []error
	for _, v := range Validations {
		r := v(vCtx)
		if r.Allowed {
			continue
		}
		errs = append(errs, errors.New(r.Reason))
	}
	return utilerrors.NewAggregate(errs)
}

// ValidationHandler exposes Elasticsearch validations as an admission.Handler.
type ValidationHandler struct {
	client  client.Client
	decoder types.Decoder
}

var _ inject.Client = &ValidationHandler{}

// Handle processes AdmissionRequests.
func (v *ValidationHandler) Handle(ctx context.Context, r types.Request) types.Response {
	if r.AdmissionRequest.Operation == v1beta1.Delete {
		return admission.ValidationResponse(true, "allowing all deletes")
	}
	esCluster := estype.Elasticsearch{}
	log.Info("ValidationHandler handler called",
		"operation", r.AdmissionRequest.Operation,
		"name", r.AdmissionRequest.Name,
		"namespace", r.AdmissionRequest.Namespace,
	)
	err := v.decoder.Decode(r, &esCluster)
	if err != nil {
		log.Error(err, "Failed to decode request")
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	var onServer estype.Elasticsearch
	err = v.client.Get(ctx, k8s.ExtractNamespacedName(&esCluster), &onServer)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to retrieve existing cluster")
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	var current *estype.Elasticsearch
	if err == nil {
		current = &onServer
	}
	var results []ValidationResult
	validationCtx, err := NewValidationContext(current, esCluster)
	if err != nil {
		log.Error(err, "while creating validation context")
		return admission.ValidationResponse(false, err.Error())
	}
	for _, v := range Validations {
		results = append(results, v(*validationCtx))
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
