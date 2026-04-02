// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// WebhookPath is the HTTP path for the Elasticsearch validating webhook.
	WebhookPath = "/validate-elasticsearch-k8s-elastic-co-v1beta1-elasticsearch"
)

// +kubebuilder:webhook:path=/validate-elasticsearch-k8s-elastic-co-v1beta1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1beta1,name=elastic-es-validation-v1beta1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var eslog = ulog.Log.WithName("es-validation")

// Validate validates an Elasticsearch resource, optionally against an old version for update validations.
func Validate(es *Elasticsearch, old *Elasticsearch) (admission.Warnings, error) {
	eslog.V(1).Info("validate", "name", es.Name)

	var (
		errs     field.ErrorList
		warnings admission.Warnings
	)

	deprecationWarning, deprecationErrors := commonv1.CheckDeprecatedStackVersion(es.Spec.Version)
	if len(deprecationErrors) > 0 {
		errs = append(errs, deprecationErrors...)
	}
	if deprecationWarning != "" {
		warnings = append(warnings, deprecationWarning)
	}
	settingWarns, settingErrs := settingsWarningsAndErrors(es)
	warnings = append(warnings, settingWarns...)
	errs = append(errs, settingErrs...)

	if old != nil {
		for _, val := range updateValidations {
			if err := val(old, es); err != nil {
				errs = append(errs, err...)
			}
		}
		if len(errs) > 0 {
			return warnings, apierrors.NewInvalid(
				schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
				es.Name, errs)
		}
	}

	if validationErrs := es.check(validations); len(validationErrs) > 0 {
		errs = append(errs, validationErrs...)
	}

	if len(errs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			es.Name, errs)
	}
	return warnings, nil
}
