// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package compare

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelsAndAnnotationsAreEqual compares just the labels and annotations for equality from two ObjectMeta instances.
func LabelsAndAnnotationsAreEqual(a, b metav1.ObjectMeta) bool {
	return reflect.DeepEqual(a.Labels, b.Labels) && reflect.DeepEqual(a.Annotations, b.Annotations)
}

// EqualsSemantically compares two objects while ignoring unset fields
func EqualsSemantically(t *testing.T, want, have interface{}) {
	t.Helper()

	if !equality.Semantic.DeepDerivative(want, have) {
		// Produce a nice diff
		JSONEqual(t, want, have)
	}
}
