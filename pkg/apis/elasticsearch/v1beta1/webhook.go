// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/validate-elasticsearch-k8s-elastic-co-v1beta1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1beta1,name=elastic-es-validation-v1beta1.k8s.elastic.co

func (es *Elasticsearch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(es).
		Complete()
}

var eslog = logf.Log.WithName("es-validation")

var _ webhook.Validator = &Elasticsearch{}

func (es *Elasticsearch) ValidateCreate() error {
	eslog.V(1).Info("validate create", "name", es.Name)
	return es.validateElasticsearch()
}

// ValidateDelete is required to implement webhook.Validator, but we do not actually validate deletes
func (es *Elasticsearch) ValidateDelete() error {
	return nil
}

func (es *Elasticsearch) ValidateUpdate(old runtime.Object) error {
	eslog.V(1).Info("validate update", "name", es.Name)
	oldEs, ok := old.(*Elasticsearch)
	if !ok {
		return errors.New("cannot cast old object to Elasticsearch type")
	}

	var errs field.ErrorList
	for _, val := range updateValidations {
		if err := val(oldEs, es); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			es.Name, errs)
	}
	return es.validateElasticsearch()
}

func (es *Elasticsearch) validateElasticsearch() error {
	errs := es.check(validations)
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			es.Name,
			errs,
		)
	}
	return nil
}
