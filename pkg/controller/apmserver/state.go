// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
)

// State holds the accumulated state during the reconcile loop including the response and a pointer to an ApmServer
// resource for status updates.
type State struct {
	ApmServer         *apmv1.ApmServer
	originalApmServer *apmv1.ApmServer
}

// NewState creates a new reconcile state based on the given request and ApmServer resource, with the
// ApmServer's Status.ObservedGeneration set from the current generation of the ApmServer's specification.
func NewState(as *apmv1.ApmServer) State {
	current := as.DeepCopy()
	current.Status.ObservedGeneration = as.Generation
	return State{ApmServer: current, originalApmServer: as}
}

// UpdateApmServerState updates the ApmServer status based on the given deployment.
func (s State) UpdateApmServerState(ctx context.Context, deployment v1.Deployment, pods []corev1.Pod, apmServerSecret corev1.Secret) error {
	deploymentStatus, err := common.DeploymentStatus(ctx, s.ApmServer.Status.DeploymentStatus, deployment, pods, APMVersionLabelName)
	if err != nil {
		return err
	}
	s.ApmServer.Status.DeploymentStatus = deploymentStatus
	s.ApmServer.Status.SecretTokenSecretName = apmServerSecret.Name
	return nil
}

// UpdateApmServerExternalService updates the ApmServer ExternalService status.
func (s State) UpdateApmServerExternalService(svc corev1.Service) {
	s.ApmServer.Status.ExternalService = svc.Name
}
