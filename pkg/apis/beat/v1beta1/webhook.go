// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: "Beat"}
	validationLog = logf.Log.WithName("beat-v1beta1-validation")

	defaultChecks = []func(*Beat) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAtMostOneDeploymentOption,
		checkImageIfTypeUnknown,
	}

	updateChecks = []func(old, curr *Beat) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-beat-k8s-elastic-co-v1beta1-beat,mutating=false,failurePolicy=ignore,groups=beat.k8s.elastic.co,resources=beats,verbs=create;update,versions=v1beta1,name=elastic-beat-validation-v1beta1.k8s.elastic.co

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

func checkNoUnknownFields(b *Beat) field.ErrorList {
	return commonv1.NoUnknownFields(b, b.ObjectMeta)
}

func checkNameLength(ent *Beat) field.ErrorList {
	return commonv1.CheckNameLength(ent)
}

func checkSupportedVersion(b *Beat) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(b.Spec.Version, version.SupportedBeatVersions)
}

func checkAtMostOneDeploymentOption(b *Beat) field.ErrorList {
	if b.Spec.DaemonSet != nil && b.Spec.Deployment != nil {
		msg := fmt.Sprintf("Specify either daemonSet or deployment, not both")
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("daemonSet"), msg),
			field.Forbidden(field.NewPath("spec").Child("deployment"), msg),
		}
	}

	return nil
}

func checkImageIfTypeUnknown(b *Beat) field.ErrorList {
	knownTypes := []string{"filebeat", "metricbeat"}
	if !stringsutil.StringInSlice(b.Spec.Type, knownTypes) &&
		b.Spec.Image == "" {
		return field.ErrorList{
			field.Required(
				field.NewPath("spec").Child("image"),
				"Image is required if Beat type is not well known."),
		}
	}
	return nil
}

func checkNoDowngrade(prev, curr *Beat) field.ErrorList {
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}
