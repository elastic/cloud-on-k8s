// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	pkgerrors "github.com/pkg/errors"
)

const (
	cfgInvalidMsg            = "configuration invalid"
	nameTooLongErrMsg        = "Elasticsearch name length cannot exceed the limit of %d characters"
	masterRequiredMsg        = "Elasticsearch needs to have at least one master node"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
	invalidSanIPErrMsg       = "invalid SAN IP address"
	pvcImmutableMsg          = "Volume claim templates cannot be modified"
)

// Validation is a function from a currently stored Elasticsearch spec and proposed new spec
// (both inside a Context struct) to a validation.Result.
type Validation func(ctx Context) validation.Result

// ElasticsearchVersion groups an ES resource and its parsed version.
type ElasticsearchVersion struct {
	Elasticsearch estype.Elasticsearch
	Version       version.Version
}

// Context is structured input for validation functions.
type Context struct {
	// Current is the Elasticsearch spec/version currently stored in the api server. Can be nil on new clusters.
	Current *ElasticsearchVersion
	// Proposed is the Elasticsearch spec/version submitted for validation.
	Proposed ElasticsearchVersion
}

// NewValidationContext constructs a new Context.
func NewValidationContext(current *estype.Elasticsearch, proposed estype.Elasticsearch) (*Context, error) {
	proposedVersion, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		return nil, pkgerrors.Wrap(err, parseVersionErrMsg)
	}
	ctx := Context{
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

func (v Context) isCreate() bool {
	return v.Current == nil
}

// Validate runs validation logic in contexts where we don't have current and proposed Elasticsearch versions.
func Validate(es estype.Elasticsearch) ([]validation.Result, error) {
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}

	vCtx := Context{
		Current: nil,
		Proposed: ElasticsearchVersion{
			Elasticsearch: es,
			Version:       *v,
		},
	}
	var errs []validation.Result
	for _, v := range Validations {
		r := v(vCtx)
		if r.Allowed {
			continue
		}
		errs = append(errs, r)
	}
	return errs, nil
}
