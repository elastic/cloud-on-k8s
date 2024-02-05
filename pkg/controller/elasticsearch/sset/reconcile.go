// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// ReconcileStatefulSet creates or updates the expected StatefulSet.
func ReconcileStatefulSet(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, expected appsv1.StatefulSet, expectations *expectations.Expectations) (appsv1.StatefulSet, error) {
	podTemplateValidator := statefulset.NewPodTemplateValidator(ctx, c, &es, expected)
	var reconciled appsv1.StatefulSet
	err := reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// expected labels or annotations not there
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				// different spec
				!EqualTemplateHashLabels(expected, reconciled)
		},
		UpdateReconciled: func() {
			// override annotations and labels with expected ones
			// don't remove additional values in reconciled that may have been defaulted or
			// manually set by the user on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Spec = expected.Spec
		},
		PreCreate: podTemplateValidator,
		PreUpdate: podTemplateValidator,
		PostUpdate: func() {
			if expectations != nil {
				// expect the reconciled StatefulSet to be there in the cache for next reconciliations,
				// to prevent assumptions based on the wrong replica count
				expectations.ExpectGeneration(reconciled)
			}
		},
	})
	return reconciled, err
}

// EqualTemplateHashLabels reports whether actual and expected StatefulSets have the same template hash label value.
func EqualTemplateHashLabels(expected, actual appsv1.StatefulSet) bool {
	return expected.Labels[hash.TemplateHashLabelName] == actual.Labels[hash.TemplateHashLabelName]
}
