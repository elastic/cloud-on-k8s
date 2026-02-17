// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook/admission"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Enterprise Search validating webhook.
	webhookPath = "/validate-enterprisesearch-k8s-elastic-co-v1beta1-enterprisesearch"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("enterprisesearch-v1beta1-validation")

	defaultChecks = []func(*EnterpriseSearch) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}

	updateChecks = []func(old, curr *EnterpriseSearch) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-enterprisesearch-k8s-elastic-co-v1beta1-enterprisesearch,mutating=false,failurePolicy=ignore,groups=enterprisesearch.k8s.elastic.co,resources=enterprisesearches,verbs=create;update,versions=v1beta1,name=elastic-ent-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var _ admission.Validator = (*EnterpriseSearch)(nil)

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (ent *EnterpriseSearch) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", ent.Name)
	return ent.validate(nil)
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (ent *EnterpriseSearch) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", ent.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (ent *EnterpriseSearch) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", ent.Name)
	oldObj, ok := old.(*EnterpriseSearch)
	if !ok {
		return nil, errors.New("cannot cast old object to EnterpriseSearch type")
	}

	return ent.validate(oldObj)
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (ent *EnterpriseSearch) WebhookPath() string {
	return webhookPath
}

func (ent *EnterpriseSearch) validate(old *EnterpriseSearch) (admission.Warnings, error) {
	var errors field.ErrorList
	var warnings admission.Warnings

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
