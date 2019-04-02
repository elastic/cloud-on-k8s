// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package operator

// Roles that an operator can assume
const (
	// NamespaceOperator manages applications in a single namespace
	NamespaceOperator = "namespace"
	// GlobalOperator manages cross-namespace resources (licensing, CCS, CCR, etc.)
	GlobalOperator = "global"
	// WebhookServer runs validation and mutation webhooks for the operator.
	WebhookServer = "webhook"
	// All combines all roles
	All = "all"
)

var Roles = map[string]struct{}{
	NamespaceOperator: {},
	GlobalOperator:    {},
	WebhookServer:     {},
	All:               {},
}
