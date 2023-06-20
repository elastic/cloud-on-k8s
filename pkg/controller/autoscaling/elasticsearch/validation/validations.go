// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/autoscaling"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

type validation func(autoscaler v1alpha1.ElasticsearchAutoscaler) (field.ErrorList, error)

// validations are the validation funcs that apply to creates or updates
func validations(ctx context.Context, k8sClient k8s.Client, checker license.Checker) []validation {
	return []validation{
		func(proposed v1alpha1.ElasticsearchAutoscaler) (field.ErrorList, error) {
			return validLicenseLevel(ctx, proposed, checker)
		},
		noUnknownFields,
		validName,
		func(proposed v1alpha1.ElasticsearchAutoscaler) (field.ErrorList, error) {
			return validAutoscalingConfiguration(ctx, proposed, k8sClient)
		},
		func(proposed v1alpha1.ElasticsearchAutoscaler) (field.ErrorList, error) {
			return noAutoscalingAnnotation(ctx, proposed, k8sClient)
		},
	}
}

var (
	// autoscalingSpecPath helps to compute the path used in validation error fields.
	autoscalingSpecPath = func(index int, child string, moreChildren ...string) *field.Path {
		return field.NewPath("spec").
			Child("policies").
			Index(index).
			Child(child, moreChildren...)
	}
)

func noAutoscalingAnnotation(ctx context.Context, esa v1alpha1.ElasticsearchAutoscaler, k8sClient k8s.Client) (field.ErrorList, error) {
	var errs field.ErrorList
	// Fetch the associated Elasticsearch resource
	var es esv1.Elasticsearch
	esNamespacedName := types.NamespacedName{Name: esa.Spec.ElasticsearchRef.Name, Namespace: esa.Namespace}
	if err := k8sClient.Get(ctx, esNamespacedName, &es); err != nil {
		if apierrors.IsNotFound(err) {
			esalog.Info("associated Elasticsearch not found")
			return errs, err
		}
		esalog.Info("error while getting the associated Elasticsearch resource, skipping validation", "error", err.Error())
		return errs, err
	}
	if es.IsAutoscalingAnnotationSet() {
		errs = append(
			errs, field.Invalid(
				field.NewPath("metadata").Child("annotations", esv1.ElasticsearchAutoscalingSpecAnnotationName),
				esv1.ElasticsearchAutoscalingSpecAnnotationName,
				"Autoscaling annotation is no longer supported, please remove the annotation",
			),
		)
	}
	return errs, nil
}

func validAutoscalingConfiguration(ctx context.Context, esa v1alpha1.ElasticsearchAutoscaler, k8sClient k8s.Client) (field.ErrorList, error) {
	var errs field.ErrorList
	// Validate the autoscaling policies
	errs = append(errs, autoscaling.ValidateAutoscalingPolicies(autoscalingSpecPath, esa.Spec.AutoscalingPolicySpecs)...)
	if len(errs) > 0 {
		// We may have policies with duplicated set of roles, it may make it hard to validate further the autoscaling spec.
		return errs, nil
	}

	// Fetch the associated Elasticsearch resource
	var es esv1.Elasticsearch
	esNamespacedName := types.NamespacedName{Name: esa.Spec.ElasticsearchRef.Name, Namespace: esa.Namespace}
	if err := k8sClient.Get(ctx, esNamespacedName, &es); err != nil {
		if apierrors.IsNotFound(err) {
			esalog.Info("associated Elasticsearch not found")
			return errs, err
		}
		esalog.Info("error while getting the associated Elasticsearch resource, skipping validation", "error", err.Error())
		return errs, err
	}
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		esalog.Info("error while parsing Elasticsearch version, skipping validation", "error", err.Error())
		return errs, err
	}
	errs = append(errs, autoscaling.ValidateAutoscalingSpecification(autoscalingSpecPath, esa.Spec.AutoscalingPolicySpecs, es, ver)...)
	return errs, nil
}

// validName checks whether the name is valid.
func validName(esa v1alpha1.ElasticsearchAutoscaler) (field.ErrorList, error) {
	if len(esa.Name) > common_name.MaxResourceNameLength {
		return field.ErrorList{field.TooLong(field.NewPath("metadata").Child("name"), esa.Name, common_name.MaxResourceNameLength)}, nil
	}
	return nil, nil
}

// noUnknownFields checks whether the last applied config annotation contains json with unknown fields.
func noUnknownFields(esa v1alpha1.ElasticsearchAutoscaler) (field.ErrorList, error) {
	return commonv1.NoUnknownFields(&esa, esa.ObjectMeta), nil
}

func validLicenseLevel(ctx context.Context, esa v1alpha1.ElasticsearchAutoscaler, checker license.Checker) (field.ErrorList, error) {
	var errs field.ErrorList
	ok, err := license.HasRequestedLicenseLevel(ctx, esa.Annotations, checker)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "while checking license level during validation")
		return errs, nil // ignore the error here
	}
	if !ok {
		errs = append(errs, field.Invalid(field.NewPath("metadata").Child("annotations").Child(license.Annotation), "enterprise", "Enterprise license required but ECK operator is running on a Basic license"))
	}
	return errs, nil
}
