// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
)

// State holds the accumulated state during the reconcile loop including the response and a pointer to an ApmServer
// resource for status updates.
type State struct {
	ApmServer *apmv1.ApmServer
	Result    reconcile.Result
	Request   reconcile.Request

	originalApmServer *apmv1.ApmServer
}

// NewState creates a new reconcile state based on the given request and ApmServer resource with the resource
// state reset to empty.
func NewState(request reconcile.Request, as *apmv1.ApmServer) State {
	return State{Request: request, ApmServer: as, originalApmServer: as.DeepCopy()}
}

// UpdateApmServerState updates the ApmServer status based on the given deployment.
func (s State) UpdateApmServerState(deployment v1.Deployment, pods []corev1.Pod, apmServerSecret corev1.Secret) {
	s.ApmServer.Status.DeploymentStatus = common.DeploymentStatus(s.ApmServer.Status.DeploymentStatus, deployment, pods, APMVersionLabelName)
	s.ApmServer.Status.SecretTokenSecretName = apmServerSecret.Name
}

// UpdateApmServerExternalService updates the ApmServer ExternalService status.
func (s State) UpdateApmServerExternalService(svc corev1.Service) {
	s.ApmServer.Status.ExternalService = svc.Name
}
