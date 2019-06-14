// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package operator

import (
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/about"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Parameters contain parameters to create new operators.
type Parameters struct {
	// OperatorClient is a Kubernetes client configured in the operator namespace and not in the managed namespace
	OperatorClient client.Client
	// OperatorImage is the operator docker image. The operator needs to be aware of its image to use it in sidecars.
	OperatorImage string
	// OperatorNamespace is the control plane namespace of the operator.
	OperatorNamespace string
	// OperatorInfo is information about the operator
	OperatorInfo about.OperatorInfo
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
	// CACertValidity is the validity duration of a newly created CA cert
	CACertValidity time.Duration
	// CACertRotateBefore defines how long before expiration CA certificates should be rotated
	CACertRotateBefore time.Duration
	// CertValidity is the validity duration of a newly created certificate
	CertValidity time.Duration
	// CertRotateBefore defines how long before expiration certificates should be rotated
	CertRotateBefore time.Duration
}
