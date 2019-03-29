// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"net/http"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	gherror "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	"k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const masterRequiredMsg = "Elasticsearch needs to have at least one master node"

var log = logf.Log.WithName("es-validation")

// Validation is a function from a currently stored Elasticsearch spec and proposed new spec to a ValidationResult.
type Validation func(ctx ValidationContext) ValidationResult

// ValidationResult contains validation results.
type ValidationResult struct {
	Error   error
	Allowed bool
	Reason  string
}

// ElasticsearchVersion groups an ES resource and its parsed version.
type ElasticsearchVersion struct {
	Elasticsearch estype.Elasticsearch
	Version       version.Version
}

// ValidationContext is structured input for validation functions.
type ValidationContext struct {
	Current  *ElasticsearchVersion
	Proposed ElasticsearchVersion
}

// NewValidationContext constructs a new ValidationContext.
func NewValidationContext(current *estype.Elasticsearch, proposed estype.Elasticsearch) (*ValidationContext, error) {
	proposedVersion, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		return nil, gherror.Wrap(err, parseVersionErrMsg)
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
			return nil, gherror.Wrap(err, parseStoredVersionErrMsg)
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

var OK = ValidationResult{Allowed: true}

// Validations are all registered Elasticsearch validations.
var Validations = []Validation{
	hasMaster,
	supportedVersion,
	noDowngrades,
	validUpgradePath,
}

func supportedVersion(ctx ValidationContext) ValidationResult {
	if v := driver.SupportedVersions(ctx.Proposed.Version); v == nil {
		return ValidationResult{Allowed: false, Reason: unsupportedVersion(&ctx.Proposed.Version)}
	}
	return OK
}

// hasMaster checks if the given Elasticsearch cluster has at least one master node.
func hasMaster(ctx ValidationContext) ValidationResult {
	var hasMaster bool
	for _, t := range ctx.Proposed.Elasticsearch.Spec.Topology {
		hasMaster = hasMaster || (t.NodeTypes.Master && t.NodeCount > 0)
	}
	if hasMaster {
		return OK
	}
	return ValidationResult{Reason: masterRequiredMsg}
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
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	var current *estype.Elasticsearch
	err = v.client.Get(ctx, k8s.ExtractNamespacedName(&esCluster), current)
	if errors.IsNotFound(err) {
		current = nil
	} else if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
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
