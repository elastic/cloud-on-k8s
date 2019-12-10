// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
)

// State holds the accumulated state during the reconcile loop including the response and a pointer to a Kibana
// resource for status updates.
type State struct {
	Kibana  *kbv1.Kibana
	Request reconcile.Request

	originalKibana *kbv1.Kibana
}

// NewState creates a new reconcile state based on the given request and Kibana resource with the resource
// state reset to empty.
func NewState(request reconcile.Request, kb *kbv1.Kibana) State {
	return State{Request: request, Kibana: kb, originalKibana: kb.DeepCopy()}
}

// UpdateKibanaState updates the Kibana status based on the given deployment.
func (s State) UpdateKibanaState(deployment appsv1.Deployment) {
	s.Kibana.Status.AvailableNodes = deployment.Status.AvailableReplicas
	s.Kibana.Status.Health = kbv1.KibanaRed
	for _, c := range deployment.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			s.Kibana.Status.Health = kbv1.KibanaGreen
		}
	}
}
