// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	corev1 "k8s.io/api/core/v1"
)

// DefaultSecurityContext returns a minimalist, restricted, security context.
// Values should be inherited and checked against a PSP, but we provide some
// default values if pods are started outside E2E tests, by a developer for example.
func DefaultSecurityContext() *corev1.PodSecurityContext {
	dscc := &corev1.PodSecurityContext{
		RunAsNonRoot: BoolPtr(true),
	}

	if !Ctx().OcpCluster {
		defaultUserID := int64(12345) // arbitrary user ID
		// Stack images expected to run with the user ID 1000 or 0 before 7.9.0 for APM and Beats (https://github.com/elastic/beats/issues/18871)
		// and 7.11.0 for Enterprise Search (https://github.com/elastic/enterprise-search-team/issues/285)
		stackVersion := version.MustParse(Ctx().ElasticStackVersion)
		if stackVersion.LT(version.MustParse("7.11.0")) {
			defaultUserID = int64(1000)
		}
		dscc.RunAsUser = &defaultUserID
		dscc.FSGroup = &defaultUserID
	}

	return dscc
}

// It's currently not possible to run APM using OpenShift's
// restricted SCC. Therefore, we are forcing the required UID
// and fsGroup for APM's security context. A dedicated ServiceAccount
// with special permissions is created by APM test's builder
// so that this can work.
func APMDefaultSecurityContext() *corev1.PodSecurityContext {
	defaultUserID := int64(1000)

	return &corev1.PodSecurityContext{
		RunAsNonRoot: BoolPtr(true),
		RunAsUser:    &defaultUserID,
		FSGroup:      &defaultUserID,
	}
}
