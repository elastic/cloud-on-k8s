// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReconcileStatefulSet creates or updates the expected StatefulSet.
func ReconcileStatefulSet(c k8s.Client, scheme *runtime.Scheme, es v1beta1.Elasticsearch, expected appsv1.StatefulSet, expectations *expectations.Expectations) (appsv1.StatefulSet, error) {
	var reconciled appsv1.StatefulSet
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			if len(reconciled.Labels) == 0 {
				return true
			}
			return !EqualTemplateHashLabels(expected, reconciled)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(&reconciled)
		},
		PostUpdate: func() {
			if expectations != nil {
				// expect the reconciled StatefulSet to be there in the cache for next reconciliations,
				// to prevent assumptions based on the wrong replica count
				expectations.ExpectGeneration(reconciled.ObjectMeta)
			}
		},
	})
	return reconciled, err
}

// EqualTemplateHashLabels reports whether actual and expected StatefulSets have the same template hash label value.
func EqualTemplateHashLabels(expected, actual appsv1.StatefulSet) bool {
	return expected.Labels[hash.TemplateHashLabelName] == actual.Labels[hash.TemplateHashLabelName]
}
