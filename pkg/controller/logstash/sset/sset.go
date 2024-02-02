// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

type Params struct {
	Name                 string
	Namespace            string
	ServiceName          string
	Selector             map[string]string
	Labels               map[string]string
	PodTemplateSpec      corev1.PodTemplateSpec
	UpdateStrategy       appsv1.StatefulSetUpdateStrategy
	VolumeClaimTemplates []corev1.PersistentVolumeClaim
	Replicas             int32
	RevisionHistoryLimit *int32
}

func New(params Params) appsv1.StatefulSet {
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: params.UpdateStrategy,
			// we don't care much about pods creation ordering, and manage deletion ordering ourselves,
			// so we're fine with the StatefulSet controller spawning all pods in parallel
			PodManagementPolicy:  appsv1.ParallelPodManagement,
			RevisionHistoryLimit: params.RevisionHistoryLimit,
			// build a headless service per StatefulSet, matching the StatefulSet labels
			ServiceName: params.ServiceName,
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},

			Replicas:             &params.Replicas,
			Template:             params.PodTemplateSpec,
			VolumeClaimTemplates: params.VolumeClaimTemplates,
		},
	}

	// store a hash of the sset resource in its labels for comparison purposes
	sset.Labels = hash.SetTemplateHashLabel(sset.Labels, sset.Spec)

	return sset
}

// Reconcile creates or updates the expected StatefulSet.
func Reconcile(ctx context.Context, c k8s.Client, expected appsv1.StatefulSet, ls lsv1alpha1.Logstash, expectations *expectations.Expectations) (appsv1.StatefulSet, error) {
	var reconciled appsv1.StatefulSet
	podTemplateValidator := statefulset.NewPodTemplateValidator(ctx, c, &ls, expected)
	err := reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      &ls,
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
