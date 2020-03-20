// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReconcileSecret creates or updates the actual secret to match the expected one.
// Existing annotations or labels that are not expected are preserved.
func ReconcileSecret(c k8s.Client, expected corev1.Secret, owner metav1.Object) (corev1.Secret, error) {
	var reconciled corev1.Secret
	if err := ReconcileResource(Params{
		Client:     c,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// update if expected labels and annotations are not there
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				// or if secret data is not strictly equal
				!reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			// set expected annotations and labels, but don't remove existing ones
			// that may have been defaulted or set by the user on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
		},
	}); err != nil {
		return corev1.Secret{}, err
	}
	return reconciled, nil
}
