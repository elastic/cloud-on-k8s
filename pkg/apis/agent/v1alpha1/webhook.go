// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Elastic Agent validating webhook.
	webhookPath = "/validate-agent-k8s-elastic-co-v1alpha1-agent"

	MissingPolicyIDMessage = "spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("agent-v1alpha1-validation")
)

// +kubebuilder:webhook:path=/validate-agent-k8s-elastic-co-v1alpha1-agent,mutating=false,failurePolicy=ignore,groups=agent.k8s.elastic.co,resources=agents,verbs=create;update,versions=v1alpha1,name=elastic-agent-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &Agent{}

func (a *Agent) GetWarnings() []string {
	if a == nil {
		return nil
	}
	if a.Spec.Mode == AgentFleetMode && len(a.Spec.PolicyID) == 0 {
		return []string{fmt.Sprintf("%s %s/%s: %s", Kind, a.Namespace, a.Name, MissingPolicyIDMessage)}
	}
	return nil
}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (a *Agent) ValidateCreate() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate create", "name", a.Name)
	return a.validate(nil)
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (a *Agent) ValidateDelete() (admission.Warnings, error) {
	validationLog.V(1).Info("Validate delete", "name", a.Name)
	return nil, nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (a *Agent) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	validationLog.V(1).Info("Validate update", "name", a.Name)
	oldObj, ok := old.(*Agent)
	if !ok {
		return nil, errors.New("cannot cast old object to Agent type")
	}

	return a.validate(oldObj)
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (a *Agent) WebhookPath() string {
	return webhookPath
}

func (a *Agent) validate(old *Agent) (admission.Warnings, error) {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, a); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return nil, apierrors.NewInvalid(groupKind, a.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(a); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return nil, apierrors.NewInvalid(groupKind, a.Name, errors)
	}
	return nil, nil
}
