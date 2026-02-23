// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

type validation func(*autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList

// validations returns the set of validation functions to run, including license-aware checks.
func validations(ctx context.Context, checker license.Checker) []validation {
	return []validation{
		checkNoUnknownFields,
		checkNameLength,
		func(policy *autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList {
			return checkSupportedVersion(ctx, policy, checker)
		},
		checkConfigSecretName,
		checkResourceSelector,
		func(policy *autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList {
			return checkLicenseLevel(ctx, policy, checker)
		},
	}
}

func checkNoUnknownFields(policy *autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList {
	return commonv1.NoUnknownFields(policy, policy.ObjectMeta)
}

func checkNameLength(policy *autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList {
	return commonv1.CheckNameLength(policy)
}

// checkSupportedVersion validates the version against license-dependent minimums.
// Enterprise license holders may use versions starting from 9.2.1, while non-enterprise
// users must use 9.2.4 or later.
func checkSupportedVersion(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy, checker license.Checker) field.ErrorList {
	supported := version.SupportedAutoOpsAgentNonEnterpriseVersions
	enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		// In the case of failure while checking enterprise features during version validation, we log
		// the error and return the error to retry the reconciliation.
		ulog.FromContext(ctx).Error(err, "while checking enterprise features during version validation")
		return field.ErrorList{field.InternalError(field.NewPath("spec").Child("version"), err)}
	}
	if enabled {
		supported = version.SupportedAutoOpsAgentEnterpriseVersions
	}
	return commonv1.CheckSupportedStackVersion(policy.Spec.Version, supported)
}

func checkConfigSecretName(policy *autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList {
	if policy.Spec.AutoOpsRef.SecretName == "" {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("autoOpsRef").Child("secretName"), "AutoOpsRef secret name must be specified")}
	}
	return nil
}

func checkResourceSelector(policy *autoopsv1alpha1.AutoOpsAgentPolicy) field.ErrorList {
	if policy.Spec.ResourceSelector.MatchLabels == nil && len(policy.Spec.ResourceSelector.MatchExpressions) == 0 {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("resourceSelector"), "ResourceSelector must be specified with either matchLabels or matchExpressions")}
	}
	return nil
}

// checkLicenseLevel validates that the operator has the requested license level
// as indicated by the eck.k8s.elastic.co/license annotation.
func checkLicenseLevel(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy, checker license.Checker) field.ErrorList {
	ok, err := license.HasRequestedLicenseLevel(ctx, policy.Annotations, checker)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "while checking license level during validation")
		return nil
	}
	if !ok {
		return field.ErrorList{field.Invalid(
			field.NewPath("metadata").Child("annotations").Child(license.Annotation),
			"enterprise",
			"Enterprise license required but ECK operator is running on a Basic license",
		)}
	}
	return nil
}
