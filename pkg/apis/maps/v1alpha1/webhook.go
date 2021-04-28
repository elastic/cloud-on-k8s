// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
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
	validationLog = ulog.Log.WithName("maps-v1alpha1-validation")

	defaultChecks = []func(*ElasticMapsServer) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
	}
)

// +kubebuilder:webhook:path=/validate-ems-k8s-elastic-co-v1alpha1-mapsservers,mutating=false,failurePolicy=ignore,groups=maps.k8s.elastic.co,resources=mapsservers,verbs=create;update,versions=v1alpha1,name=elastic-ems-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1alpha1,matchPolicy=Exact

var _ webhook.Validator = &ElasticMapsServer{}

func (m *ElasticMapsServer) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

func (m *ElasticMapsServer) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", m.Name)
	return m.validate()
}

func (m *ElasticMapsServer) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", m.Name)
	return nil
}

func (m *ElasticMapsServer) ValidateUpdate(_ runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", m.Name)
	return m.validate()
}

func (m *ElasticMapsServer) validate() error {
	var errors field.ErrorList

	for _, dc := range defaultChecks {
		if err := dc(m); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		validationLog.V(1).Info("failed validation", "errors", errors)
		return apierrors.NewInvalid(groupKind, m.Name, errors)
	}
	return nil
}

func checkNoUnknownFields(k *ElasticMapsServer) field.ErrorList {
	return commonv1.NoUnknownFields(k, k.ObjectMeta)
}

func checkNameLength(k *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckNameLength(k)
}

func checkSupportedVersion(k *ElasticMapsServer) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(k.Spec.Version, version.SupportedMapsVersions)
}
