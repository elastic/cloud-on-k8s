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
