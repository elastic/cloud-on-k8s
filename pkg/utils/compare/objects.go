// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package compare

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelsAndAnnotationsAreEqual compares just the labels and annotations for equality from two ObjectMeta instances.
func LabelsAndAnnotationsAreEqual(a, b metav1.ObjectMeta) bool {
	return reflect.DeepEqual(a.Labels, b.Labels) && reflect.DeepEqual(a.Annotations, b.Annotations)
}
