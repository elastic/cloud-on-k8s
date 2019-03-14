// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package operator

import (
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
)

// Parameters contain parameters to create new operators.
type Parameters struct {
	// OperatorImage is the operator docker image. The operator needs to be aware of its image to use it in sidecars.
	OperatorImage string
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
	// CACertValidity is the validity duration of a newly created CA cert
	CACertValidity time.Duration
	// CACertRotateBefore defines how long before expiration certificates should be rotated
	CACertRotateBefore time.Duration
}
