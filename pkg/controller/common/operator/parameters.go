// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package operator

import (
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

// Parameters contain parameters to create new operators.
type Parameters struct {
	// OperatorNamespace is the control plane namespace of the operator.
	OperatorNamespace string
	// OperatorInfo is information about the operator
	OperatorInfo about.OperatorInfo
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
	// IPFamily represents the IP family to use when creating configuration and services.
	IPFamily corev1.IPFamily
	// CACertRotation defines the rotation params for CA certificates.
	CACertRotation certificates.RotationParams
	// CertRotation defines the rotation params for non-CA certificates.
	CertRotation certificates.RotationParams
	// MaxConcurrentReconciles controls the number of goroutines per controller.
	MaxConcurrentReconciles int
	// SetDefaultSecurityContext enables setting the default security context
	// with fsGroup=1000 for Elasticsearch 8.0+ Pods. Ignored pre-8.0
	SetDefaultSecurityContext bool
	// ValidateStorageClass specifies whether storage classes volume expansion support should be verified.
	// Can be disabled if cluster-wide storage class RBAC access is not available.
	ValidateStorageClass bool
	// Tracer is a shared APM tracer instance or nil
	Tracer *apm.Tracer
}
