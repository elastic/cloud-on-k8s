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

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	// webhookPath is the HTTP path for the Elasticsearch validating webhook.
	webhookPath = "/validate-elasticsearch-k8s-elastic-co-v1beta1-elasticsearch"
)

// +kubebuilder:webhook:path=/validate-elasticsearch-k8s-elastic-co-v1beta1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1beta1,name=elastic-es-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

var eslog = ulog.Log.WithName("es-validation")

var _ webhook.Validator = &Elasticsearch{}

// ValidateCreate is called by the validating webhook to validate the create operation.
// Satisfies the webhook.Validator interface.
func (es *Elasticsearch) ValidateCreate() error {
	eslog.V(1).Info("validate create", "name", es.Name)
	return es.validateElasticsearch()
}

// ValidateDelete is required to implement webhook.Validator, but we do not actually validate deletes.
func (es *Elasticsearch) ValidateDelete() error {
	return nil
}

// ValidateUpdate is called by the validating webhook to validate the update operation.
// Satisfies the webhook.Validator interface.
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

// WebhookPath returns the HTTP path used by the validating webhook.
func (es *Elasticsearch) WebhookPath() string {
	return webhookPath
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
