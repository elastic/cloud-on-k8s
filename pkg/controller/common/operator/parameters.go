// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	esvalidation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

// Parameters contain parameters to create new operators.
type Parameters struct {
	// ElasticsearchObservationInterval is the interval between (asynchronous) observations of Elasticsearch health.
	ElasticsearchObservationInterval time.Duration
	// ExposedNodeLabels holds regular expressions of node labels which are allowed to be automatically set as annotations on Elasticsearch Pods.
	ExposedNodeLabels esvalidation.NodeLabels
	// OperatorNamespace is the control plane namespace of the operator.
	OperatorNamespace string
	// OperatorInfo is information about the operator
	OperatorInfo about.OperatorInfo
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
	// PasswordHasher is the password hash generator used by the operator.
	PasswordHasher cryptutil.PasswordHasher
	// IPFamily represents the IP family to use when creating configuration and services.
	IPFamily corev1.IPFamily
	// GlobalCA is an optionally configured, globally shared CA to be used for all managed resources.
	GlobalCA *certificates.CA
	// CACertRotation defines the rotation params for CA certificates.
	CACertRotation certificates.RotationParams
	// CertRotation defines the rotation params for non-CA certificates.
	CertRotation certificates.RotationParams
	// MaxConcurrentReconciles controls the number of goroutines per controller.
	MaxConcurrentReconciles int
	// SetDefaultSecurityContext enables setting the default security context
	// with fsGroup=1000 for Elasticsearch 8.0+ Pods. Ignored pre-8.0
	SetDefaultSecurityContext bool
	// ValidateStorageClass specifies whether the operator should retrieve storage classes to verify volume expansion support.
	// Can be disabled if cluster-wide storage class RBAC access is not available.
	ValidateStorageClass bool
	// Tracer is a shared APM tracer instance or nil
	Tracer *apm.Tracer
}
