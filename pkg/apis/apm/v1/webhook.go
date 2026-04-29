// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

const (
	// WebhookPath is the HTTP path for the APM Server validating webhook.
	WebhookPath = "/validate-apm-k8s-elastic-co-v1-apmserver"
)

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	// ApmAgentConfigurationMinVersion is the minimum required version to establish an association with Kibana
	ApmAgentConfigurationMinVersion = version.MustParse("7.5.1")

	defaultChecks = []func(*ApmServer) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAgentConfigurationMinVersion,
		checkAssociations,
	}

	updateChecks = []func(old, curr *ApmServer) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-apm-k8s-elastic-co-v1-apmserver,mutating=false,failurePolicy=ignore,groups=apm.k8s.elastic.co,resources=apmservers,verbs=create;update,versions=v1,name=elastic-apm-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate is called by the validating webhook to validate the create or update operation.
func Validate(as *ApmServer, old *ApmServer) (admission.Warnings, error) {
	return as.validate(old)
}

func (as *ApmServer) validate(old *ApmServer) (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

	deprecationWarnings, deprecationErrors := checkIfVersionDeprecated(as)
	if deprecationErrors != nil {
		errors = append(errors, deprecationErrors...)
	}
	if deprecationWarnings != "" {
		warnings = append(warnings, deprecationWarnings)
	}
	if resourcesWarning := commonv1.PodTemplateResourcesOverrideWarning(
		"spec.resources",
		"spec.podTemplate",
		ApmServerContainerName,
		as.Spec.Resources,
		as.Spec.PodTemplate,
	); resourcesWarning != "" {
		warnings = append(warnings, resourcesWarning)
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

func checkAgentConfigurationMinVersion(as *ApmServer) field.ErrorList {
	if !as.Spec.KibanaRef.IsSet() {
		return nil
	}
	apmVersion, err := commonv1.ParseVersion(as.EffectiveVersion())
	if err != nil {
		return err
	}
	if !apmVersion.GTE(ApmAgentConfigurationMinVersion) {
		return field.ErrorList{field.Forbidden(
			field.NewPath("spec").Child("kibanaRef"),
			fmt.Sprintf(
				"minimum required version for Kibana association is %s but desired version is %s",
				ApmAgentConfigurationMinVersion,
				apmVersion,
			),
		),
		}
	}
	return nil
}

func checkAssociations(as *ApmServer) field.ErrorList {
	err1 := commonv1.CheckElasticsearchSelectorRefs(field.NewPath("spec").Child("elasticsearchRef"), as.Spec.ElasticsearchRef)
	err2 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("kibanaRef"), as.Spec.KibanaRef)
	return append(err1, err2...)
}
