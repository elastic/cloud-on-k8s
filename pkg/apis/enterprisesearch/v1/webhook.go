// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

const (
	// WebhookPath is the HTTP path for the Enterprise Search validating webhook.
	WebhookPath = "/validate-enterprisesearch-k8s-elastic-co-v1-enterprisesearch"
)

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	defaultChecks = []func(*EnterpriseSearch) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAssociation,
	}

	updateChecks = []func(old, curr *EnterpriseSearch) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-enterprisesearch-k8s-elastic-co-v1-enterprisesearch,mutating=false,failurePolicy=ignore,groups=enterprisesearch.k8s.elastic.co,resources=enterprisesearches,verbs=create;update,versions=v1,name=elastic-ent-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate validates an EnterpriseSearch resource, taking the old object for update validation.
func Validate(ent *EnterpriseSearch, old *EnterpriseSearch) (admission.Warnings, error) {
	return ent.validate(old)
}

func (ent *EnterpriseSearch) validate(old *EnterpriseSearch) (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

	// check if the version is deprecated
	deprecationWarnings, deprecationErrors := checkIfVersionDeprecated(ent)
	if deprecationErrors != nil {
		errors = append(errors, deprecationErrors...)
	}
	if deprecationWarnings != "" {
		warnings = append(warnings, deprecationWarnings)
	}

	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, ent); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return warnings, apierrors.NewInvalid(groupKind, ent.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(ent); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, ent.Name, errors)
	}
	return warnings, nil
}

func checkNoUnknownFields(ent *EnterpriseSearch) field.ErrorList {
	return commonv1.NoUnknownFields(ent, ent.ObjectMeta)
}

func checkNameLength(ent *EnterpriseSearch) field.ErrorList {
	return commonv1.CheckNameLength(ent)
}

func checkSupportedVersion(ent *EnterpriseSearch) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(ent.Spec.Version, version.SupportedEnterpriseSearchVersions)
}

func checkIfVersionDeprecated(ent *EnterpriseSearch) (string, field.ErrorList) {
	return commonv1.CheckDeprecatedStackVersion(ent.Spec.Version)
}

func checkNoDowngrade(prev, curr *EnterpriseSearch) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkAssociation(ent *EnterpriseSearch) field.ErrorList {
	return commonv1.CheckElasticsearchSelectorRefs(field.NewPath("spec").Child("elasticsearchRef"), ent.Spec.ElasticsearchRef)
}
