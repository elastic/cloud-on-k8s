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
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the StackConfigPolicy validating webhook.
	webhookPath = "/validate-scp-k8s-elastic-co-v1alpha1-stackconfigpolicies"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("scp-v1alpha1-validation")

	defaultChecks = []func(*StackConfigPolicy) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		validSettings,
	}
)

// +kubebuilder:webhook:path=/validate-scp-k8s-elastic-co-v1alpha1-stackconfigpolicies,mutating=false,failurePolicy=ignore,groups=stackconfigpolicy.k8s.elastic.co,resources=stackconfigpolicies,verbs=create;update,versions=v1alpha1,name=elastic-scp-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &StackConfigPolicy{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (p *StackConfigPolicy) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", p.Name)
	return p.validate()
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (p *StackConfigPolicy) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", p.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (p *StackConfigPolicy) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", p.Name)
	return p.validate()
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (p *StackConfigPolicy) WebhookPath() string {
	return webhookPath
}

func (p *StackConfigPolicy) validate() (admission.Warnings, error) {
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

func checkNoUnknownFields(policy *StackConfigPolicy) field.ErrorList {
	return commonv1.NoUnknownFields(policy, policy.ObjectMeta)
}

func checkNameLength(policy *StackConfigPolicy) field.ErrorList {
	return commonv1.CheckNameLength(policy)
}

func validSettings(policy *StackConfigPolicy) field.ErrorList {
	settingsCount := 0
	if policy.Spec.Elasticsearch.ClusterSettings != nil {
		settingsCount += len(policy.Spec.Elasticsearch.ClusterSettings.Data)
	}
	if policy.Spec.Elasticsearch.SnapshotRepositories != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SnapshotRepositories.Data)
	}
	if policy.Spec.Elasticsearch.SnapshotLifecyclePolicies != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SnapshotLifecyclePolicies.Data)
	}
	if policy.Spec.Elasticsearch.SecurityRoleMappings != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SecurityRoleMappings.Data)
	}
	if policy.Spec.Elasticsearch.IndexLifecyclePolicies != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IndexLifecyclePolicies.Data)
	}
	if policy.Spec.Elasticsearch.IngestPipelines != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IngestPipelines.Data)
	}
	if policy.Spec.Elasticsearch.IndexTemplates.ComponentTemplates != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IndexTemplates.ComponentTemplates.Data)
	}
	if policy.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates.Data)
	}
	if settingsCount == 0 {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("elasticsearch"), "Elasticsearch settings are mandatory and must not be empty")}
	}
	return nil
}
