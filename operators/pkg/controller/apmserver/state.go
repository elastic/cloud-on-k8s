// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// State holds the accumulated state during the reconcile loop including the response and a pointer to an ApmServer
// resource for status updates.
type State struct {
	ApmServer *v1alpha1.ApmServer
	Result    reconcile.Result
	Request   reconcile.Request

	originalApmServer *v1alpha1.ApmServer
}

// NewState creates a new reconcile state based on the given request and ApmServer resource with the resource
// state reset to empty.
func NewState(request reconcile.Request, as *v1alpha1.ApmServer) State {
	return State{Request: request, ApmServer: as, originalApmServer: as.DeepCopy()}
}

// UpdateApmServerState updates the ApmServer status based on the given deployment.
func (s State) UpdateApmServerState(deployment v1.Deployment, apmServerSecret corev1.Secret) {
	s.ApmServer.Status.SecretTokenSecretName = apmServerSecret.Name
	s.ApmServer.Status.AvailableNodes = int(deployment.Status.AvailableReplicas) // TODO lossy type conversion
	s.ApmServer.Status.Health = v1alpha1.ApmServerRed
	for _, c := range deployment.Status.Conditions {
		if c.Type == v1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			s.ApmServer.Status.Health = v1alpha1.ApmServerGreen
		}
	}
}

// UpdateApmServerExternalService updates the ApmServer ExternalService status.
func (s State) UpdateApmServerExternalService(svc corev1.Service) {
	s.ApmServer.Status.ExternalService = svc.Name
}

func (s *State) UpdateApmServerControllerVersion(version string) {
	s.ApmServer.Status.ControllerVersion = version
}

func (s *State) GetApmServerControllerVersion() string {
	return s.ApmServer.Status.ControllerVersion
}
