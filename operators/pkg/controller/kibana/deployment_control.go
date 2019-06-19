// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	defaultRevisionHistoryLimit int32 = 0
)

type DeploymentParams struct {
	Name            string
	Namespace       string
	Selector        map[string]string
	Labels          map[string]string
	Replicas        int32
	PodTemplateSpec corev1.PodTemplateSpec
}

// NewDeployment creates a Deployment API struct with the given PodSpec.
func NewDeployment(params DeploymentParams) appsv1.Deployment {
	return withTemplateHash(appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: common.Int32(defaultRevisionHistoryLimit),
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},
			Template: params.PodTemplateSpec,
			Replicas: &params.Replicas,
		},
	})
}

// withTemplateHash stores a hash of the deployment in labels to ease deployment comparisons
func withTemplateHash(d appsv1.Deployment) appsv1.Deployment {
	d.Labels = hash.SetTemplateHashLabel(d.Labels, d)
	return d
}

// ReconcileDeployment upserts the given deployment for the specified owner.
func ReconcileDeployment(c k8s.Client, s *runtime.Scheme, expected appsv1.Deployment, owner metav1.Object) (appsv1.Deployment, error) {
	reconciled := &appsv1.Deployment{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return ShouldUpdateDeployment(expected, *reconciled)
		},
		UpdateReconciled: func() {
			// Update the found object and write the result back if there are any changes
			reconciled.Labels = expected.Labels
			reconciled.Annotations = expected.Annotations
			reconciled.Spec = expected.Spec
		},
	})
	return *reconciled, err
}

// ShouldUpdateDeployment returns true if both expected and actual have the same deployment template hash.
// This ensures the new expected deployment would in fact lead to the exact same actual deployment,
// but allows user to customize existing deployment labels or annotations after creation, without
// triggering a new deployment to be rolled out.
func ShouldUpdateDeployment(expected appsv1.Deployment, actual appsv1.Deployment) bool {
	// deployment template hash should be the exact same
	return hash.GetTemplateHashLabel(expected.Labels) != hash.GetTemplateHashLabel(actual.Labels)
}
