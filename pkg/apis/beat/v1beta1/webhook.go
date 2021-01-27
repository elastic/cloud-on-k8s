// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

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
	validationLog = ulog.Log.WithName("beat-v1beta1-validation")
)

// +kubebuilder:webhook:path=/validate-beat-k8s-elastic-co-v1beta1-beat,mutating=false,failurePolicy=ignore,groups=beat.k8s.elastic.co,resources=beats,verbs=create;update,versions=v1beta1,name=elastic-beat-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &Beat{}

func (b *Beat) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(b).
		Complete()
}

func (b *Beat) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", b.Name)
	return b.validate(nil)
}

func (b *Beat) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", b.Name)
	return nil
}

func (b *Beat) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", b.Name)
	oldObj, ok := old.(*Beat)
	if !ok {
		return errors.New("cannot cast old object to Beat type")
	}

	return b.validate(oldObj)
}

func (b *Beat) validate(old *Beat) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, b); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return apierrors.NewInvalid(groupKind, b.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(b); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return apierrors.NewInvalid(groupKind, b.Name, errors)
	}
	return nil
}
