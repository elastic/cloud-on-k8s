// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Parameters are params for the webhook server.
type Parameters struct {
	// Bootstrap are bootstrap options for the webhook.
	Bootstrap webhook.BootstrapOptions
	// AutoInstall controls whether the operator will try to install the webhook and supporting resources itself.
	AutoInstall bool
}

// BootstrapOptionsParams are params to create webhook BootstrapOptions.
type BootstrapOptionsParams struct {
	Namespace        string
	ManagedNamespace string
	SecretName       string
	ServiceSelector  string
}

// NewBootstrapOptions are options for the webhook bootstrap process.
func NewBootstrapOptions(params BootstrapOptionsParams) webhook.BootstrapOptions {
	var secret *types.NamespacedName
	ns := params.Namespace
	if params.ManagedNamespace != "" {
		// if we are restricting the operator to a single namespace we have to create the webhook resources in the
		// managed namespace due to restrictions in the controller runtime (would not be able to list the resources)
		ns = params.ManagedNamespace
	}
	if params.SecretName != "" {
		secret = &types.NamespacedName{
			Namespace: ns,
			Name:      params.SecretName,
		}
	}
	var svc *webhook.Service
	if params.ServiceSelector != "" {
		svc = &webhook.Service{
			Namespace: ns,
			Name:      svcName,
			Selectors: map[string]string{
				controlPlane: params.ServiceSelector,
			},
		}
	}
	return webhook.BootstrapOptions{
		Secret:  secret,
		Service: svc,
	}
}
