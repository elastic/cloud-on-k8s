// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Elastic Logstash validating webhook.
	webhookPath = "/validate-logstash-k8s-elastic-co-v1alpha1-logstash"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("logstash-v1alpha1-validation")
)

// +kubebuilder:webhook:path=/validate-logstash-k8s-elastic-co-v1alpha1-logstash,mutating=false,failurePolicy=ignore,groups=logstash.k8s.elastic.co,resources=logstashes,verbs=create;update,versions=v1alpha1,name=elastic-logstash-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &Logstash{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (a *Logstash) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", a.Name)
	return a.validate(nil)
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (a *Logstash) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", a.Name)
	return nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (a *Logstash) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", a.Name)
	oldObj, ok := old.(*Logstash)
	if !ok {
		return errors.New("cannot cast old object to Logstash type")
	}

	return a.validate(oldObj)
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (a *Logstash) WebhookPath() string {
	return webhookPath
}

func (a *Logstash) validate(old *Logstash) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, a); err != nil {
				errors = append(errors, err...)
			}
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
