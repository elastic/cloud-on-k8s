// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Elastic Maps Server validating webhook.
	webhookPath = "/validate-ems-k8s-elastic-co-v1alpha1-mapsservers"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("maps-v1alpha1-validation")

	defaultChecks = []func(*ElasticMapsServer) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAssociation,
	}
)

// +kubebuilder:webhook:path=/validate-ems-k8s-elastic-co-v1alpha1-mapsservers,mutating=false,failurePolicy=ignore,groups=maps.k8s.elastic.co,resources=mapsservers,verbs=create;update,versions=v1alpha1,name=elastic-ems-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &ElasticMapsServer{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (m *ElasticMapsServer) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", m.Name)
	return m.validate()
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (m *ElasticMapsServer) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", m.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (m *ElasticMapsServer) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", m.Name)
	return m.validate()
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (m *ElasticMapsServer) WebhookPath() string {
	return webhookPath
}

func (m *ElasticMapsServer) validate() (admission.Warnings, error) {
	var errors field.ErrorList

	for _, dc := range defaultChecks {
		if err := dc(m); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		validationLog.V(1).Info("failed validation", "errors", errors)
		return nil, apierrors.NewInvalid(groupKind, m.Name, errors)
	}
	return nil, nil
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

func checkAssociation(ems *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckAssociationRefs(field.NewPath("spec").Child("elasticsearchRef"), ems.Spec.ElasticsearchRef)
}
