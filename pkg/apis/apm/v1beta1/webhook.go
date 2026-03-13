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
	// WebhookPath is the HTTP path for the APM Server validating webhook.
	WebhookPath = "/validate-apm-k8s-elastic-co-v1beta1-apmserver"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: "ApmServer"}
	validationLog = ulog.Log.WithName("apm-v1beta1-validation")

	defaultChecks = []func(*ApmServer) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}

	updateChecks = []func(old, curr *ApmServer) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-apm-k8s-elastic-co-v1beta1-apmserver,mutating=false,failurePolicy=ignore,groups=apm.k8s.elastic.co,resources=apmservers,verbs=create;update,versions=v1beta1,name=elastic-apm-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate is called by the validating webhook to validate the ApmServer resource.
func Validate(as *ApmServer, old *ApmServer) (admission.Warnings, error) {
	return as.validate(old)
}

func (as *ApmServer) validate(old *ApmServer) (admission.Warnings, error) {
	var errors field.ErrorList
	var warnings admission.Warnings

	// depreciation check
	depreciationWarnings, depreciationErrors := checkIfVersionDeprecated(as)
	if depreciationErrors != nil {
		errors = append(errors, depreciationErrors...)
	}
	if depreciationWarnings != "" {
		warnings = append(warnings, depreciationWarnings)
	}

	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, as); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return warnings, apierrors.NewInvalid(groupKind, as.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(as); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, as.Name, errors)
	}
	return warnings, nil
}

func checkNoUnknownFields(as *ApmServer) field.ErrorList {
	return commonv1.NoUnknownFields(as, as.ObjectMeta)
}

func checkNameLength(as *ApmServer) field.ErrorList {
	return commonv1.CheckNameLength(as)
}

func checkSupportedVersion(as *ApmServer) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(as.Spec.Version, version.SupportedAPMServerVersions)
}

func checkIfVersionDeprecated(as *ApmServer) (string, field.ErrorList) {
	return commonv1.CheckDeprecatedStackVersion(as.Spec.Version)
}

func checkNoDowngrade(prev, curr *ApmServer) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}
