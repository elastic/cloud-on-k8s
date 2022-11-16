// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

// DefaultSecurityContext returns a minimalist, restricted, security context.
func DefaultSecurityContext() *corev1.PodSecurityContext {
	// OpenShift sets the security context automatically
	if Ctx().OcpCluster {
		return &corev1.PodSecurityContext{
			RunAsNonRoot: BoolPtr(true),
		}
	}

	// if not OpenShift set an arbitrary user ID
	defaultUserID := int64(12345)
	// use 1000 before 7.11.0 as it was the only accepted non 0 user ID accepted by Enterprise Search (https://github.com/elastic/enterprise-search-team/issues/285),
	// APM and Beats images have been fixed in 7.9.0 (https://github.com/elastic/beats/issues/18871)
	if version.MustParse(Ctx().ElasticStackVersion).LT(version.MustParse("7.11.0")) {
		defaultUserID = int64(1000)
	}
	return &corev1.PodSecurityContext{
		RunAsNonRoot: BoolPtr(true),
		RunAsUser:    &defaultUserID,
		FSGroup:      &defaultUserID,
	}
}
