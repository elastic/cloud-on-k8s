// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"errors"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	groupKind     = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}
	validationLog = ulog.Log.WithName("apm-v1-validation")

	// ApmAgentConfigurationMinVersion is the minimum required version to establish an association with Kibana
	ApmAgentConfigurationMinVersion = version.MustParse("7.5.1")

	defaultChecks = []func(*ApmServer) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAgentConfigurationMinVersion,
	}

	updateChecks = []func(old, curr *ApmServer) field.ErrorList{
		checkNoDowngrade,
	}
)

// +kubebuilder:webhook:path=/validate-apm-k8s-elastic-co-v1-apmserver,mutating=false,failurePolicy=ignore,groups=apm.k8s.elastic.co,resources=apmservers,verbs=create;update,versions=v1,name=elastic-apm-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &ApmServer{}

func (as *ApmServer) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(as).
		Complete()
}

func (as *ApmServer) ValidateCreate() error {
	validationLog.V(1).Info("Validate create", "name", as.Name)
	return as.validate(nil)
}

func (as *ApmServer) ValidateDelete() error {
	validationLog.V(1).Info("Validate delete", "name", as.Name)
	return nil
}

func (as *ApmServer) ValidateUpdate(old runtime.Object) error {
	validationLog.V(1).Info("Validate update", "name", as.Name)
	oldObj, ok := old.(*ApmServer)
	if !ok {
		return errors.New("cannot cast old object to ApmServer type")
	}

	return as.validate(oldObj)
}

func (as *ApmServer) validate(old *ApmServer) error {
	var errors field.ErrorList
	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, as); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return apierrors.NewInvalid(groupKind, as.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(as); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return apierrors.NewInvalid(groupKind, as.Name, errors)
	}
	return nil
}

func checkNoUnknownFields(as *ApmServer) field.ErrorList {
	return commonv1.NoUnknownFields(as, as.ObjectMeta)
}

func checkNameLength(as *ApmServer) field.ErrorList {
	return commonv1.CheckNameLength(as)
}

func checkSupportedVersion(as *ApmServer) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(as.Spec.Version, version.SupportedAPMServerVersions)
}

func checkNoDowngrade(prev, curr *ApmServer) field.ErrorList {
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkAgentConfigurationMinVersion(as *ApmServer) field.ErrorList {
	if !as.Spec.KibanaRef.IsDefined() {
		return nil
	}
	apmVersion, err := commonv1.ParseVersion(as.EffectiveVersion())
	if err != nil {
		return err
	}
	if !apmVersion.IsSameOrAfter(ApmAgentConfigurationMinVersion) {
		return field.ErrorList{field.Forbidden(
			field.NewPath("spec").Child("kibanaRef"),
			fmt.Sprintf(
				"minimum required version for Kibana association is %s but desired version is %s",
				ApmAgentConfigurationMinVersion,
				apmVersion,
			),
		),
		}

	}
	return nil
}
