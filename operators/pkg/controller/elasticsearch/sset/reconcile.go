// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func ReconcileStatefulSet(c k8s.Client, scheme *runtime.Scheme, es v1alpha1.Elasticsearch, expected appsv1.StatefulSet) error {
	var reconciled appsv1.StatefulSet
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			if len(reconciled.Labels) == 0 {
				return true
			}
			return expected.Labels[hash.TemplateHashLabelName] != reconciled.Labels[hash.TemplateHashLabelName]
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(&reconciled)
		},
	})
}
