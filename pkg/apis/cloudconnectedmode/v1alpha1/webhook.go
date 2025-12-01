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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook/admission"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the CloudConnected validating webhook.
	webhookPath                  = "/validate-ccm-k8s-elastic-co-v1alpha1-cloudconnectedmodes"
	SpecSecureSettingsDeprecated = "spec.SecureSettings is deprecated, secure settings must be set per application"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("ccm-v1alpha1-validation")

	defaultChecks = []func(*CloudConnectedMode) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		validSettings,
	}
)

// +kubebuilder:webhook:path=/validate-ccm-k8s-elastic-co-v1alpha1-cloudconnectedmodes,mutating=false,failurePolicy=ignore,groups=cloudconnected.k8s.elastic.co,resources=cloudconnectedmodes,verbs=create;update,versions=v1alpha1,name=elastic-ccm-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var _ admission.Validator = &CloudConnectedMode{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (p *CloudConnectedMode) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", p.Name)
	return p.validate()
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (p *CloudConnectedMode) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", p.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (p *CloudConnectedMode) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", p.Name)
	return p.validate()
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (p *CloudConnectedMode) WebhookPath() string {
	return webhookPath
}

func (p *CloudConnectedMode) validate() (admission.Warnings, error) {
	var errors field.ErrorList

	for _, dc := range defaultChecks {
		if err := dc(p); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		validationLog.V(1).Info("failed validation", "errors", errors)
		return nil, apierrors.NewInvalid(groupKind, p.Name, errors)
	}
	return nil, nil
}

func (p *CloudConnectedMode) GetWarnings() []string {
	if p == nil {
		return nil
	}
	return nil
}

func checkNoUnknownFields(policy *CloudConnectedMode) field.ErrorList {
	return commonv1.NoUnknownFields(policy, policy.ObjectMeta)
}

func checkNameLength(policy *CloudConnectedMode) field.ErrorList {
	return commonv1.CheckNameLength(policy)
}

func validSettings(policy *CloudConnectedMode) field.ErrorList {
	// Validate that ResourceSelector is not empty
	if policy.Spec.ResourceSelector.MatchLabels == nil && len(policy.Spec.ResourceSelector.MatchExpressions) == 0 {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("resourceSelector"), "ResourceSelector must be specified with either matchLabels or matchExpressions")}
	}
	return nil
}
