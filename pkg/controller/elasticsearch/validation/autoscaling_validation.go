// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

func validAutoscalingConfiguration(es esv1.Elasticsearch) field.ErrorList {
	// We no longer support autoscaling annotation, return error if present
	if es.IsAutoscalingAnnotationSet() {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("metadata").Child("annotations", esv1.ElasticsearchAutoscalingSpecAnnotationName),
				esv1.ElasticsearchAutoscalingSpecAnnotationName,
				autoscalingAnnotationUnsupportedErrMsg,
			),
		}
	}
	return nil
}
