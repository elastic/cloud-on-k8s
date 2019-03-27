// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import "sigs.k8s.io/controller-runtime/pkg/webhook"

// Parameters are params for the webhook server.
type Parameters struct {
	// Bootstrap are bootstrap options for the webhook.
	Bootstrap webhook.BootstrapOptions
	// AutoInstall controls whether the operator will try to install the webhook and supporting resources itself.
	AutoInstall bool
}
