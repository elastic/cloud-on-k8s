// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
)

// Validate runs all StackConfigPolicy validation checks, including the namespace-scoping rule for
// variablesFrom sources. Namespace-scoped policies (not in the operator namespace) may only
// reference sources in their own namespace.
func Validate(p *policyv1alpha1.StackConfigPolicy, operatorNamespace string) (admission.Warnings, error) {
	warnings, err := policyv1alpha1.Validate(p, nil)
	if err != nil {
		return warnings, err
	}

	path := field.NewPath("spec").Child("variablesFrom")
	var errs field.ErrorList
	for i, src := range p.Spec.VariablesFrom {
		if !src.AllowedFrom(p.Namespace, operatorNamespace) {
			errs = append(errs, field.Forbidden(path.Index(i).Child("namespace"),
				fmt.Sprintf("namespace %q is not allowed: cross-namespace sources are only permitted for policies in the operator namespace",
					src.EffectiveNamespace(p.Namespace))))
		}
	}
	if len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: policyv1alpha1.GroupVersion.Group, Kind: policyv1alpha1.Kind},
			p.Name, errs)
	}
	return warnings, nil
}
