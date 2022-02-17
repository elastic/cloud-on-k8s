// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Enterprise Search validating webhook.
	webhookPath = "/validate-enterprisesearch-k8s-elastic-co-v1-enterprisesearch"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("enterprisesearch-v1-validation")

	defaultChecks = []func(*EnterpriseSearch) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}

	updateChecks = []func(old, curr *EnterpriseSearch) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-enterprisesearch-k8s-elastic-co-v1-enterprisesearch,mutating=false,failurePolicy=ignore,groups=enterprisesearch.k8s.elastic.co,resources=enterprisesearches,verbs=create;update,versions=v1,name=elastic-ent-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &EnterpriseSearch{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (ent *EnterpriseSearch) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", ent.Name)
	return ent.validate(nil)
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (ent *EnterpriseSearch) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", ent.Name)
	return nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (ent *EnterpriseSearch) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", ent.Name)
	oldObj, ok := old.(*EnterpriseSearch)
	if !ok {
		return errors.New("cannot cast old object to EnterpriseSearch type")
	}

	return ent.validate(oldObj)
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (ent *EnterpriseSearch) WebhookPath() string {
	return webhookPath
}

func (ent *EnterpriseSearch) validate(old *EnterpriseSearch) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, ent); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return apierrors.NewInvalid(groupKind, ent.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(ent); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return apierrors.NewInvalid(groupKind, ent.Name, errors)
	}
	return nil
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

func checkNoDowngrade(prev, curr *EnterpriseSearch) field.ErrorList {
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}
