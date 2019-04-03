// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"errors"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/validation"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("es-validation")

var Validations = []Validation{
	eulaAccepted,
}

func eulaAccepted(ctx Context) validation.Result {
	if ctx.Proposed.Spec.Eula.Accepted != true {
		return validation.Result{Allowed: false, Reason: "Please set the field eula.accepted to true to accept the EULA"}
	}
	return validation.OK
}

// Validation is a function from a currently stored Elasticsearch spec and proposed new spec
// (both inside a Context struct) to a Result.
type Validation func(ctx Context) validation.Result

// Context is structured input for validation functions.
type Context struct {
	// Current is the EnterpriseLicense  stored in the api server. Can be nil on new clusters.
	Current *estype.EnterpriseLicense
	// Proposed is the EnterpriseLicense submitted for validation.
	Proposed estype.EnterpriseLicense
}

// NewValidationContext constructs a new Context.
func NewValidationContext(current *estype.EnterpriseLicense, proposed estype.EnterpriseLicense) (*Context, error) {

	ctx := Context{
		Proposed: proposed,
	}
	if current != nil {
		ctx.Current = current
	}
	return &ctx, nil
}

func (v Context) isCreate() bool {
	return v.Current == nil
}

// Validate runs validation logic in contexts where we don't have current and proposed Elasticsearch versions.
func Validate(es estype.EnterpriseLicense) error {

	vCtx := Context{
		Current:  nil,
		Proposed: es,
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
