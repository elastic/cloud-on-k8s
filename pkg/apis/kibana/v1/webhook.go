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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

const (
	// WebhookPath is the HTTP path for the Kibana validating webhook.
	WebhookPath = "/validate-kibana-k8s-elastic-co-v1-kibana"
)

// +kubebuilder:webhook:path=/validate-kibana-k8s-elastic-co-v1-kibana,mutating=false,failurePolicy=ignore,groups=kibana.k8s.elastic.co,resources=kibanas,verbs=create;update,versions=v1,name=elastic-kb-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	defaultChecks = []func(*Kibana) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkMonitoring,
		checkAssociations,
	}

	updateChecks = []func(old, curr *Kibana) field.ErrorList{
		checkNoDowngrade,
	}
)

// Validate validates a Kibana resource. old is nil on create.
func Validate(k *Kibana, old *Kibana) (admission.Warnings, error) {
	return k.validate(old)
}

func (k *Kibana) validate(old *Kibana) (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

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

func checkMonitoring(k *Kibana) field.ErrorList {
	errs := validations.Validate(k, k.Spec.Version, validations.MinStackVersion)
	// Kibana must be associated to an Elasticsearch when monitoring metrics are enabled
	if monitoring.IsMetricsDefined(k) && !k.Spec.ElasticsearchRef.IsSet() {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("elasticsearchRef"), k.Spec.ElasticsearchRef,
			validations.InvalidKibanaElasticsearchRefForStackMonitoringMsg))
	}
	return errs
}

func checkAssociations(k *Kibana) field.ErrorList {
	monitoringPath := field.NewPath("spec").Child("monitoring")
	err1 := commonv1.CheckAssociationRefs(monitoringPath.Child("metrics"), k.GetMonitoringMetricsRefs()...)
	err2 := commonv1.CheckAssociationRefs(monitoringPath.Child("logs"), k.GetMonitoringLogsRefs()...)
	err3 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("elasticsearchRef"), k.Spec.ElasticsearchRef)
	err4 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("enterpriseSearchRef"), k.Spec.EnterpriseSearchRef)
	err5 := commonv1.CheckLocalAssociationRefs(field.NewPath("spec").Child("packageRegistryRef"), k.Spec.PackageRegistryRef)
	return append(err1, append(err2, append(err3, append(err4, err5...)...)...)...)
}
