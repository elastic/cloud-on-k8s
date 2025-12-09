// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook/admission"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the AutoOpsAgentPolicy validating webhook.
	webhookPath = "/validate-autoops-k8s-elastic-co-v1alpha1-autoopsagentpolicies"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("autoops-v1alpha1-validation")

	defaultChecks = []func(*AutoOpsAgentPolicy) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkConfigSecretName,
		validSettings,
	}
)

// +kubebuilder:webhook:path=/validate-autoops-k8s-elastic-co-v1alpha1-autoopsagentpolicies,mutating=false,failurePolicy=ignore,groups=autoops.k8s.elastic.co,resources=autoopsagentpolicies,verbs=create;update,versions=v1alpha1,name=elastic-autoops-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var _ admission.Validator = &AutoOpsAgentPolicy{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (p *AutoOpsAgentPolicy) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", p.Name)
	return p.validate()
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (p *AutoOpsAgentPolicy) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", p.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (p *AutoOpsAgentPolicy) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", p.Name)
	return p.validate()
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (p *AutoOpsAgentPolicy) WebhookPath() string {
	return webhookPath
}

func (p *AutoOpsAgentPolicy) validate() (admission.Warnings, error) {
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

func (p *AutoOpsAgentPolicy) GetWarnings() []string {
	if p == nil {
		return nil
	}
	return nil
}

func validSettings(policy *AutoOpsAgentPolicy) field.ErrorList {
	// Validate that the ResourceSelector is not empty
	if policy.Spec.ResourceSelector.MatchLabels == nil && len(policy.Spec.ResourceSelector.MatchExpressions) == 0 {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("resourceSelector"), "ResourceSelector must be specified with either matchLabels or matchExpressions")}
	}
	return nil
}
