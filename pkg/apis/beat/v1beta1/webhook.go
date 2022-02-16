// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Elastic Beats validating webhook.
	webhookPath = "/validate-beat-k8s-elastic-co-v1beta1-beat"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("beat-v1beta1-validation")
)

// +kubebuilder:webhook:path=/validate-beat-k8s-elastic-co-v1beta1-beat,mutating=false,failurePolicy=ignore,groups=beat.k8s.elastic.co,resources=beats,verbs=create;update,versions=v1beta1,name=elastic-beat-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &Beat{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (b *Beat) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", b.Name)
	return b.validate(nil)
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (b *Beat) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", b.Name)
	return nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (b *Beat) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", b.Name)
	oldObj, ok := old.(*Beat)
	if !ok {
		return errors.New("cannot cast old object to Beat type")
	}

	return b.validate(oldObj)
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (b *Beat) WebhookPath() string {
	return webhookPath
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
