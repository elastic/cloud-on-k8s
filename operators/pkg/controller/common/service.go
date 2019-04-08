// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultServiceType is used when the stack spec is empty or not valid.
	DefaultServiceType = corev1.ServiceTypeClusterIP
)

// GetServiceType obtains the service type from a string.
// There's no validation here since it is assumed to happen at the API level.
func GetServiceType(s string) corev1.ServiceType {
	switch corev1.ServiceType(s) {
	case corev1.ServiceTypeNodePort:
		return corev1.ServiceTypeNodePort
	case corev1.ServiceTypeLoadBalancer:
		return corev1.ServiceTypeLoadBalancer
	default:
		return DefaultServiceType
	}
}

// hasNodePort returns for a given service type, if the service ports have a NodePort or not.
func hasNodePort(svcType corev1.ServiceType) bool {
	return svcType == corev1.ServiceTypeNodePort || svcType == corev1.ServiceTypeLoadBalancer
}
