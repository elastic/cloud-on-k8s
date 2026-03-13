// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// WebhookPath is the HTTP path for the Elastic Beats validating webhook.
	WebhookPath = "/validate-beat-k8s-elastic-co-v1beta1-beat"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("beat-v1beta1-validation")
)

// +kubebuilder:webhook:path=/validate-beat-k8s-elastic-co-v1beta1-beat,mutating=false,failurePolicy=ignore,groups=beat.k8s.elastic.co,resources=beats,verbs=create;update,versions=v1beta1,name=elastic-beat-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate is called to validate a Beat resource.
func Validate(b *Beat, old *Beat) (admission.Warnings, error) {
	return b.validate(old)
}

func (b *Beat) validate(old *Beat) (admission.Warnings, error) {
	var errors field.ErrorList
	var warnings admission.Warnings

	// deprecation check
	deprecationWarning, deprecationError := checkIfVersionDeprecated(b)
	if deprecationError != nil {
		errors = append(errors, deprecationError...)
	}
	if deprecationWarning != "" {
		warnings = append(warnings, deprecationWarning)
	}

	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, b); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return warnings, apierrors.NewInvalid(groupKind, b.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(b); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, b.Name, errors)
	}
	return warnings, nil
}
