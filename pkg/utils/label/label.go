// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HasLabel takes in a runtime.Object and a set of label keys as parameters.
// It returns true if the object contains all the labels and false otherwise.
func HasLabel(obj metav1.Object, labels ...string) bool {
	for _, label := range labels {
		o, err := meta.Accessor(obj)
		if err != nil {
			return false
		}
		ol := o.GetLabels()
		if _, exists := ol[label]; !exists {
			return false
		}
	}
	return true
}
