// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

const (
	// WebhookPath is the HTTP path for the Elastic Package Registry validating webhook.
	WebhookPath = "/validate-epr-k8s-elastic-co-v1alpha1-packageregistry"
)

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	defaultChecks = []func(*PackageRegistry) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}
)

// +kubebuilder:webhook:path=/validate-epr-k8s-elastic-co-v1alpha1-packageregistry,mutating=false,failurePolicy=ignore,groups=packageregistry.k8s.elastic.co,resources=packageregistry,verbs=create;update,versions=v1alpha1,name=elastic-epr-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate is called to validate a PackageRegistry resource. There's no update-specific checks, so the old parameter is ignored.
func Validate(m *PackageRegistry, _ *PackageRegistry) (admission.Warnings, error) {
	return m.validate()
}

func (m *PackageRegistry) validate() (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

	for _, dc := range defaultChecks {
		if err := dc(m); err != nil {
			errors = append(errors, err...)
		}
	}

	deprecationWarning, deprecationErrors := checkIfVersionDeprecated(m)
	// No nil check needed here, as append handles this properly.
	errors = append(errors, deprecationErrors...)
	if deprecationWarning != "" {
		warnings = append(warnings, deprecationWarning)
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, m.Name, errors)
	}
	return warnings, nil
}

func checkNoUnknownFields(epr *PackageRegistry) field.ErrorList {
	return commonv1.NoUnknownFields(epr, epr.ObjectMeta)
}

func checkNameLength(epr *PackageRegistry) field.ErrorList {
	return commonv1.CheckNameLength(epr)
}

func checkSupportedVersion(epr *PackageRegistry) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(epr.Spec.Version, version.SupportedPackageRegistryVersions)
}

func checkIfVersionDeprecated(epr *PackageRegistry) (string, field.ErrorList) {
	return commonv1.CheckDeprecatedStackVersion(epr.Spec.Version)
}
