// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package configmap

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func ReconcileConfigMap(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	expected corev1.ConfigMap,
) error {
	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Client:     c,
			Scheme:     scheme,
			Owner:      &es,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Data = expected.Data
			},
		},
	)
}
