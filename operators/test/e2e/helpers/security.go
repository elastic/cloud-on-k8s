// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	corev1 "k8s.io/api/core/v1"
)

// DefaultSecurityContext returns a minimalist, restricted, security context.
// It provides some default so that pods have some descent values if they are
// started by a developer.
func DefaultSecurityContext() *corev1.PodSecurityContext {
	defaultUserId := int64(1000)
	return &corev1.PodSecurityContext{
		RunAsNonRoot: BoolPtr(true),
		RunAsUser:    &defaultUserId,
		FSGroup:      &defaultUserId,
	}
}
