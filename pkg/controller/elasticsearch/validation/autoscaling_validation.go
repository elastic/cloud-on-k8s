// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/autoscaling"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	// autoscalingSpecPath helps to compute the path used in validation error fields.
	autoscalingSpecPath = func(index int, child string, moreChildren ...string) *field.Path {
		return field.NewPath("metadata").
			Child("annotations", `"`+esv1.ElasticsearchAutoscalingSpecAnnotationName+`"`).
			Index(index).
			Child(child, moreChildren...)
	}
)

func validAutoscalingConfiguration(es esv1.Elasticsearch) field.ErrorList {
	if !es.IsAutoscalingAnnotationSet() {
		return nil
	}
	proposedVer, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg),
		}
	}

	var errs field.ErrorList
	if !proposedVer.GTE(autoscaling.ElasticsearchMinAutoscalingVersion) {
		errs = append(
			errs,
			field.Invalid(
				field.NewPath("metadata").Child("annotations", esv1.ElasticsearchAutoscalingSpecAnnotationName),
				es.Spec.Version,
				autoscalingVersionMsg,
			),
		)
		return errs
	}

	// Attempt to unmarshall the proposed autoscaling spec.
	autoscalingSpecification, err := es.GetAutoscalingSpecificationFromAnnotation()
	if err != nil {
		errs = append(errs, field.Invalid(
			field.NewPath("metadata").Child("annotations", esv1.ElasticsearchAutoscalingSpecAnnotationName),
			es.AutoscalingAnnotation(),
			err.Error(),
		))
		return errs
	}
	// Validate the autoscaling policies
	errs = append(errs, autoscaling.ValidateAutoscalingPolicies(autoscalingSpecPath, autoscalingSpecification.AutoscalingPolicySpecs)...)
	if len(errs) > 0 {
		// We may have policies with duplicated set of roles, it may make it hard to validate further the autoscaling spec.
		return errs
	}
	errs = append(errs, autoscaling.ValidateAutoscalingSpecification(autoscalingSpecPath, autoscalingSpecification.AutoscalingPolicySpecs, es, proposedVer)...)
	return errs
}
