// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package deployment

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// Params to specify a Deployment specification.
type Params struct {
	Name                 string
	Namespace            string
	Selector             map[string]string
	Labels               map[string]string
	PodTemplateSpec      corev1.PodTemplateSpec
	Replicas             int32
	RevisionHistoryLimit *int32
	Strategy             appsv1.DeploymentStrategy
}

// New creates a Deployment from the given params.
func New(params Params) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: params.RevisionHistoryLimit,
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},
			Template: params.PodTemplateSpec,
			Replicas: &params.Replicas,
			Strategy: params.Strategy,
		},
	}
}

// Reconcile creates or updates the given deployment for the specified owner.
func Reconcile(
	ctx context.Context,
	k8sClient k8s.Client,
	expected appsv1.Deployment,
	owner client.Object,
) (appsv1.Deployment, error) {
	// label the deployment with a hash of itself
	expected = WithTemplateHash(expected)

	reconciled := &appsv1.Deployment{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     k8sClient,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// compare hash of the deployment at the time it was built
			return hash.GetTemplateHashLabel(reconciled.Labels) != hash.GetTemplateHashLabel(expected.Labels)
		},
		UpdateReconciled: func() {
			// set expected annotations and labels, but don't remove existing ones
			// that may have been defaulted or set by a user/admin on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			// overwrite the spec but leave the status intact
			reconciled.Spec = expected.Spec
		},
	})
	return *reconciled, err
}

// WithTemplateHash returns a new deployment with a hash of its template to ease comparisons.
func WithTemplateHash(d appsv1.Deployment) appsv1.Deployment {
	dCopy := *d.DeepCopy()
	dCopy.Labels = hash.SetTemplateHashLabel(dCopy.Labels, dCopy)
	return dCopy
}
