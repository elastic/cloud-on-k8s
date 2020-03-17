// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
)

// State holds the accumulated state during the reconcile loop including the response and a pointer to an EnterpriseSearch
// resource for status updates.
type State struct {
	EnterpriseSearch *entsv1beta1.EnterpriseSearch
	Result           reconcile.Result
	Request          reconcile.Request

	originalEnterpriseSearch *entsv1beta1.EnterpriseSearch
}

// NewState creates a new reconcile state based on the given request and EnterpriseSearch resource with the resource
// state reset to empty.
func NewState(request reconcile.Request, ents *entsv1beta1.EnterpriseSearch) State {
	return State{Request: request, EnterpriseSearch: ents, originalEnterpriseSearch: ents.DeepCopy()}
}

// UpdateApmServerState updates the ApmServer status based on the given deployment.
func (s State) UpdateEnterpriseSearchState(deployment v1.Deployment) {
	s.EnterpriseSearch.Status.AvailableNodes = deployment.Status.AvailableReplicas
	// TODO health
}

// UpdateEnterpriseSearchExternalService updates the EnterpriseSearch ExternalService status.
func (s State) UpdateEnterpriseSearchExternalService(svc corev1.Service) {
	s.EnterpriseSearch.Status.ExternalService = svc.Name
}
