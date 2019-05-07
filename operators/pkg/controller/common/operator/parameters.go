// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package operator

import (
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
)

// Parameters contain parameters to create new operators.
type Parameters struct {
	// OperatorImage is the operator docker image. The operator needs to be aware of its image to use it in sidecars.
	OperatorImage string
	// OperatorNamespace is the control plane namespace of the operator.
	OperatorNamespace string
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
	// CACertValidity is the validity duration of a newly created CA cert
	CACertValidity time.Duration
	// CACertRotateBefore defines how long before expiration CA certificates should be rotated
	CACertRotateBefore time.Duration
	// NodeCertValidity is the validity duration of a newly created node cert
	NodeCertValidity time.Duration
	// NodeCertRotateBefore defines how long before expiration nodes certificates should be rotated
	NodeCertRotateBefore time.Duration
}
