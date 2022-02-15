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
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Kibana validating webhook.
	webhookPath = "/validate-kibana-k8s-elastic-co-v1beta1-kibana"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: "Kibana"}
	validationLog = ulog.Log.WithName("kibana-v1beta1-validation")

	defaultChecks = []func(*Kibana) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}

	updateChecks = []func(old, curr *Kibana) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-kibana-k8s-elastic-co-v1beta1-kibana,mutating=false,failurePolicy=ignore,groups=kibana.k8s.elastic.co,resources=kibanas,verbs=create;update,versions=v1beta1,name=elastic-kb-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var _ webhook.Validator = &Kibana{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (k *Kibana) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", k.Name)
	return k.validate(nil)
}

// ValidateDelete is called by the validating webhook to validate the delete operation.
// Satisfies the webhook.Validator interface.
func (k *Kibana) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", k.Name)
	return nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
func (k *Kibana) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", k.Name)
	oldObj, ok := old.(*Kibana)
	if !ok {
		return errors.New("cannot cast old object to Kibana type")
	}

	return k.validate(oldObj)
}

// WebhookPath returns the HTTP path used by the validating webhook.
func (k *Kibana) WebhookPath() string {
	return webhookPath
}

func (k *Kibana) validate(old *Kibana) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, k); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return apierrors.NewInvalid(groupKind, k.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(k); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return apierrors.NewInvalid(groupKind, k.Name, errors)
	}
	return nil
}

func checkNoUnknownFields(k *Kibana) field.ErrorList {
	return commonv1.NoUnknownFields(k, k.ObjectMeta)
}

func checkNameLength(k *Kibana) field.ErrorList {
	return commonv1.CheckNameLength(k)
}

func checkSupportedVersion(k *Kibana) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(k.Spec.Version, version.SupportedKibanaVersions)
}

func checkNoDowngrade(prev, curr *Kibana) field.ErrorList {
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}
