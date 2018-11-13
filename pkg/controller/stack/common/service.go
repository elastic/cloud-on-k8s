package common

import (
	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultServiceType is used when the stack spec is empty or not valid.
	DefaultServiceType = corev1.ServiceTypeClusterIP
)

// GetElasticsearchServiceType obtains the Elasticsearch service type as
// specified in the Expose field. There's no validation here since it happens
// at the API level.
func GetElasticsearchServiceType(s deploymentsv1alpha1.Stack) corev1.ServiceType {
	return getServiceType(s.Spec.Elasticsearch.Expose)
}

// GetKibanaServiceType obtains the Kibana service type as specified in the
// Expose field. There's no validation here since it happens at the API level.
func GetKibanaServiceType(s deploymentsv1alpha1.Stack) corev1.ServiceType {
	return getServiceType(s.Spec.Kibana.Expose)
}

func getServiceType(s string) corev1.ServiceType {
	switch corev1.ServiceType(s) {
	case corev1.ServiceTypeNodePort:
		return corev1.ServiceTypeNodePort
	case corev1.ServiceTypeLoadBalancer:
		return corev1.ServiceTypeLoadBalancer
	default:
		return DefaultServiceType
	}
}
