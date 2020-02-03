// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
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
		defaultUserID := int64(1000)
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
		RunAsUser: &defaultUserID,
		FSGroup: &defaultUserID,
	}
}
