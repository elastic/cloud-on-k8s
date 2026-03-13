// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// WebhookPath is the HTTP path for the Kibana validating webhook.
	WebhookPath = "/validate-kibana-k8s-elastic-co-v1beta1-kibana"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: "Kibana"}
	validationLog = ulog.Log.WithName("kibana-v1beta1-validation")

	defaultChecks = []func(*Kibana) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}

	updateChecks = []func(old, curr *Kibana) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-kibana-k8s-elastic-co-v1beta1-kibana,mutating=false,failurePolicy=ignore,groups=kibana.k8s.elastic.co,resources=kibanas,verbs=create;update,versions=v1beta1,name=elastic-kb-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate validates a Kibana resource, with an optional old object for update validation.
func Validate(k *Kibana, old *Kibana) (admission.Warnings, error) {
	return k.validate(old)
}

func (k *Kibana) validate(old *Kibana) (admission.Warnings, error) {
	var errors field.ErrorList
	var warnings admission.Warnings

	deprecatedWarnings, deprecatedErrors := checkIfVersionDeprecated(k)
	if len(deprecatedErrors) > 0 {
		errors = append(errors, deprecatedErrors...)
	}
	if len(deprecatedWarnings) > 0 {
		warnings = append(warnings, deprecatedWarnings)
	}

	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, k); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return warnings, apierrors.NewInvalid(groupKind, k.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(k); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, k.Name, errors)
	}
	return warnings, nil
}

func checkNoUnknownFields(k *Kibana) field.ErrorList {
	return commonv1.NoUnknownFields(k, k.ObjectMeta)
}

func checkNameLength(k *Kibana) field.ErrorList {
	return commonv1.CheckNameLength(k)
}

func checkSupportedVersion(k *Kibana) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(k.Spec.Version, version.SupportedKibanaVersions)
}

func checkIfVersionDeprecated(k *Kibana) (string, field.ErrorList) {
	return commonv1.CheckDeprecatedStackVersion(k.Spec.Version)
}

func checkNoDowngrade(prev, curr *Kibana) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}
