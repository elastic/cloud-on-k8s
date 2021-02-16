// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"errors"

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("agent-v1alpha1-validation")
)

// +kubebuilder:webhook:path=/validate-agent-k8s-elastic-co-v1alpha1-agent,mutating=false,failurePolicy=ignore,groups=agent.k8s.elastic.co,resources=agents,verbs=create;update,versions=v1alpha1,name=elastic-agent-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &Agent{}

func (a *Agent) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(a).
		Complete()
}

func (a *Agent) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", a.Name)
	return a.validate(nil)
}

func (a *Agent) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", a.Name)
	return nil
}

func (a *Agent) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", a.Name)
	oldObj, ok := old.(*Agent)
	if !ok {
		return errors.New("cannot cast old object to Agent type")
	}

	return a.validate(oldObj)
}

func (a *Agent) validate(old *Agent) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, a); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return apierrors.NewInvalid(groupKind, a.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(a); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return apierrors.NewInvalid(groupKind, a.Name, errors)
	}
	return nil
}
