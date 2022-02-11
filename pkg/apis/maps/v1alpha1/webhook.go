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

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
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
	}
)

// +kubebuilder:webhook:path=/validate-ems-k8s-elastic-co-v1alpha1-mapsservers,mutating=false,failurePolicy=ignore,groups=maps.k8s.elastic.co,resources=mapsservers,verbs=create;update,versions=v1alpha1,name=elastic-ems-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1alpha1,matchPolicy=Exact

var _ webhook.Validator = &ElasticMapsServer{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (m *ElasticMapsServer) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", m.Name)
	return m.validate()
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (m *ElasticMapsServer) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", m.Name)
	return nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (m *ElasticMapsServer) ValidateUpdate(_ runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", m.Name)
	return m.validate()
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (m *ElasticMapsServer) WebhookPath() string {
	return webhookPath
}

func (m *ElasticMapsServer) validate() error {
	var errors field.ErrorList

	for _, dc := range defaultChecks {
		if err := dc(m); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		validationLog.V(1).Info("failed validation", "errors", errors)
		return apierrors.NewInvalid(groupKind, m.Name, errors)
	}
	return nil
}

func checkNoUnknownFields(k *ElasticMapsServer) field.ErrorList {
	return commonv1.NoUnknownFields(k, k.ObjectMeta)
}

func checkNameLength(k *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckNameLength(k)
}

func checkSupportedVersion(k *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(k.Spec.Version, version.SupportedMapsVersions)
}
