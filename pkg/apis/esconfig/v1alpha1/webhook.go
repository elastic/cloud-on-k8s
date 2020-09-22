// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: "ElasticsearchConfig"}
	validationLog = logf.Log.WithName("esconfig-v1alpha1-validation")

	defaultChecks = []func(*ElasticsearchConfig) field.ErrorList{
		checkNoUnknownFields,
		validateOperations,
	}

	updateChecks = []func(old, curr *ElasticsearchConfig) field.ErrorList{}
)

// +kubebuilder:webhook:path=/validate-elasticsearchconfig-k8s-elastic-co-v1alpha1-elasticsearchconfig,mutating=false,failurePolicy=ignore,groups=elasticsearchconfig.k8s.elastic.co,resources=elasticsearchconfigs,verbs=create;update,versions=v1alpha1,name=elastic-esconfig-validation-v1alpha1.k8s.elastic.co

var _ webhook.Validator = &ElasticsearchConfig{}

func (esc *ElasticsearchConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(esc).
		Complete()
}

func (esc *ElasticsearchConfig) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "namespace", esc.Namespace, "name", esc.Name)
	return esc.validate(nil)
}

func (esc *ElasticsearchConfig) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "namespace", esc.Namespace, "name", esc.Name)
	return nil
}

func (esc *ElasticsearchConfig) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "namespace", esc.Namespace, "name", esc.Name)
	oldObj, ok := old.(*ElasticsearchConfig)
	if !ok {
		return errors.New("cannot cast old object to ElasticsearchConfig type")
	}

	return esc.validate(oldObj)
}

func (esc *ElasticsearchConfig) validate(old *ElasticsearchConfig) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, esc); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return apierrors.NewInvalid(groupKind, esc.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(esc); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return apierrors.NewInvalid(groupKind, esc.Name, errors)
	}
	return nil
}

func checkNoUnknownFields(esc *ElasticsearchConfig) field.ErrorList {
	return commonv1.NoUnknownFields(esc, esc.ObjectMeta)
}

func validateOperations(esc *ElasticsearchConfig) field.ErrorList {
	var errs field.ErrorList
	for i, op := range esc.Spec.Operations {
		// TODO also check that this has a leading /, though this could be in regex
		_, err := url.Parse(op.URL)
		if err != nil {
			errs = append(errs, field.Invalid(field.NewPath("metadata").Child("spec", "operations").Index(i), op.URL, fmt.Sprintf("cannot parse url: %s", err)))
		}
		if op.Body != "" {
			valid := json.Valid([]byte(op.Body))
			if !valid {
				errs = append(errs, field.Invalid(field.NewPath("metadata").Child("spec", "operations").Index(i), op.Body, fmt.Sprintf("cannot parse json")))
			}
		}
	}
	return errs
}
