// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook/admission"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Elastic Package Registry validating webhook.
	webhookPath = "/validate-epr-k8s-elastic-co-v1alpha1-packageregistry"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("packageregistry-v1alpha1-validation")

	defaultChecks = []func(*PackageRegistry) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}
)

// +kubebuilder:webhook:path=/validate-epr-k8s-elastic-co-v1alpha1-packageregistry,mutating=false,failurePolicy=ignore,groups=packageregistry.k8s.elastic.co,resources=packageregistry,verbs=create;update,versions=v1alpha1,name=elastic-epr-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var _ admission.Validator = (*PackageRegistry)(nil)

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (m *PackageRegistry) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", m.Name)
	return m.validate()
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (m *PackageRegistry) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", m.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (m *PackageRegistry) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", m.Name)
	return m.validate()
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (m *PackageRegistry) WebhookPath() string {
	return webhookPath
}

func (m *PackageRegistry) validate() (admission.Warnings, error) {
	var errors field.ErrorList

	for _, dc := range defaultChecks {
		if err := dc(m); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return nil, apierrors.NewInvalid(groupKind, m.Name, errors)
	}
	return nil, nil
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
