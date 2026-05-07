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
	// WebhookPath is the HTTP path for the Elastic Maps Server validating webhook.
	WebhookPath = "/validate-ems-k8s-elastic-co-v1alpha1-mapsservers"
)

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	defaultChecks = []func(*ElasticMapsServer) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAssociation,
	}
)

// +kubebuilder:webhook:path=/validate-ems-k8s-elastic-co-v1alpha1-mapsservers,mutating=false,failurePolicy=ignore,groups=maps.k8s.elastic.co,resources=elasticmapsservers,verbs=create;update,versions=v1alpha1,name=elastic-ems-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate is called by the validating webhook to validate an ElasticMapsServer resource. There's no update-specific checks, so the old parameter is ignored.
func Validate(m *ElasticMapsServer, _ *ElasticMapsServer) (admission.Warnings, error) {
	return m.validate()
}

func (m *ElasticMapsServer) validate() (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

	for _, dc := range defaultChecks {
		if err := dc(m); err != nil {
			errors = append(errors, err...)
		}
	}

	// check for deprecated version
	deprecationWarnings, deprecationErrors := checkIfVersionDeprecated(m)
	if deprecationErrors != nil {
		errors = append(errors, deprecationErrors...)
	}

	if deprecationWarnings != "" {
		warnings = append(warnings, deprecationWarnings)
	}
	if resourcesWarning := commonv1.PodTemplateResourcesOverrideWarning(
		"spec.resources",
		"spec.podTemplate",
		MapsContainerName,
		m.Spec.Resources,
		m.Spec.PodTemplate,
	); resourcesWarning != "" {
		warnings = append(warnings, resourcesWarning)
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, m.Name, errors)
	}
	return warnings, nil
}

func checkNoUnknownFields(ems *ElasticMapsServer) field.ErrorList {
	return commonv1.NoUnknownFields(ems, ems.ObjectMeta)
}

func checkNameLength(ems *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckNameLength(ems)
}

func checkSupportedVersion(ems *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(ems.Spec.Version, version.SupportedMapsVersions)
}

func checkIfVersionDeprecated(ems *ElasticMapsServer) (string, field.ErrorList) {
	return commonv1.CheckDeprecatedStackVersion(ems.Spec.Version)
}

func checkAssociation(ems *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckElasticsearchSelectorRefs(field.NewPath("spec").Child("elasticsearchRef"), ems.Spec.ElasticsearchRef)
}
