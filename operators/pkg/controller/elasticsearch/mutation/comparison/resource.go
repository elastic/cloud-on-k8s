// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// equalResourceList returns true if both ResourceList are considered equal
func equalResourceList(resListA, resListB corev1.ResourceList) bool {
	if len(resListA) != len(resListB) {
		return false
	}
	for k1, v1 := range resListA {
		if valB, ok := resListB[k1]; !ok || v1.Cmp(valB) != 0 {
			return false
		}
	}
	return true
}

// compareResources returns true if both resources match
func compareResources(actual corev1.ResourceRequirements, expected corev1.ResourceRequirements) Comparison {
	originalExpected := expected.DeepCopy()
	// need to deal with the fact actual may have defaulted values
	// we will assume for now that if expected is missing values that actual has, they will be the defaulted values
	// in effect, this will not fail a comparison if you remove limits from the spec as we cannot detect the difference
	// between a defaulted value and a missing one. moral of the story: you should definitely be explicit all the time.
	if expected.Limits == nil {
		expected.Limits = make(map[corev1.ResourceName]resource.Quantity)
	}

	for k, v := range actual.Limits {
		if _, ok := expected.Limits[k]; !ok {
			expected.Limits[k] = v
		}
	}
	if !equalResourceList(expected.Limits, actual.Limits) {
		return ComparisonMismatch(
			fmt.Sprintf("Different resource limits: expected %+v, actual %+v", expected.Limits, actual.Limits),
		)
	}

	// If Requests is omitted for a container, it defaults to Limits if that is explicitly specified
	if len(expected.Requests) == 0 {
		expected.Requests = originalExpected.Limits
	}
	if expected.Requests == nil {
		expected.Requests = make(map[corev1.ResourceName]resource.Quantity)
	}
	// see the discussion above re copying limits, which applies to defaulted requests as well
	for k, v := range actual.Requests {
		if _, ok := expected.Requests[k]; !ok {
			expected.Requests[k] = v
		}
	}
	if !equalResourceList(expected.Requests, actual.Requests) {
		return ComparisonMismatch(
			fmt.Sprintf("Different resource requests: expected %+v, actual %+v", expected.Requests, actual.Requests),
		)
	}
	return ComparisonMatch
}
