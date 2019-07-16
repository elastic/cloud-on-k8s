// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/validation"
	corev1 "k8s.io/api/core/v1"
)

const EULAValidationMsg = `Please set the annotation elastic.co/eula to "accepted" to accept the EULA`

var Validations = []Validation{
	eulaAccepted,
}

func eulaAccepted(ctx Context) validation.Result {
	if !license.IsEnterpriseTrial(ctx.Proposed) {
		return validation.OK
	}

	if ctx.Proposed.Annotations[license.EULAAnnotation] != license.EULAAcceptedValue {
		return validation.Result{Allowed: false, Reason: EULAValidationMsg}
	}
	return validation.OK
}

// Validation is a function from a currently stored Enterprise license spec and proposed new spec
// (both inside a Context struct) to a Result.
type Validation func(ctx Context) validation.Result

// Context is structured input for validation functions.
type Context struct {
	// Current is the EnterpriseLicense  stored in the api server. Can be nil on create.
	Current *corev1.Secret
	// Proposed is the EnterpriseLicense submitted for validation.
	Proposed corev1.Secret
}

// Validate runs validation logic in contexts where we don't have current and proposed EnterpriseLicenses.
func Validate(sec corev1.Secret) []validation.Result {
	vCtx := Context{
		Current:  nil,
		Proposed: sec,
	}
	var errs []validation.Result
	for _, v := range Validations {
		r := v(vCtx)
		if r.Allowed {
			continue
		}
		errs = append(errs, r)
	}
	return errs
}
