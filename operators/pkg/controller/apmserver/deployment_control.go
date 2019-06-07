// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: this file pretty much duplicates kibana/deployment_control.go

var (
	defaultRevisionHistoryLimit int32 = 0
)

type DeploymentParams struct {
	Name            string
	Namespace       string
	Selector        map[string]string
	Labels          map[string]string
	PodTemplateSpec corev1.PodTemplateSpec
	Replicas        int32
}

// NewDeployment creates a Deployment API struct with the given PodSpec.
func NewDeployment(params DeploymentParams) appsv1.Deployment {
	return appsv1.Deployment{
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
	}
}

// ReconcileDeployment upserts the given deployment for the specified owner.
func (r *ReconcileApmServer) ReconcileDeployment(expected appsv1.Deployment, owner metav1.Object) (appsv1.Deployment, error) {
	reconciled := &appsv1.Deployment{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     r.Client,
		Scheme:     r.scheme,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(expected.Spec.Selector, reconciled.Spec.Selector) ||
				!reflect.DeepEqual(expected.Spec.Replicas, reconciled.Spec.Replicas) ||
				!reflect.DeepEqual(expected.Spec.Template.ObjectMeta, reconciled.Spec.Template.ObjectMeta) ||
				!reflect.DeepEqual(expected.Spec.Template.Spec.Containers[0].Name, reconciled.Spec.Template.Spec.Containers[0].Name) ||
				!reflect.DeepEqual(expected.Spec.Template.Spec.Containers[0].Env, reconciled.Spec.Template.Spec.Containers[0].Env) ||
				!reflect.DeepEqual(expected.Spec.Template.Spec.Containers[0].Image, reconciled.Spec.Template.Spec.Containers[0].Image)
			// TODO: do something better than reflect.DeepEqual above?
			// TODO: containers[0] is a bit flaky
			// TODO: technically not only the Spec may be different, but deployment labels etc.
		},
		UpdateReconciled: func() {
			// Update the found object and write the result back if there are any changes
			reconciled.Spec = expected.Spec
		},
	})
	return *reconciled, err

}
