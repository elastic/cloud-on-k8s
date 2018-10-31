package elasticsearch

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiscoveryServiceName returns the name for the discovery service
// associated to this cluster
func DiscoveryServiceName(stackName string) string {
	return stackName + "-es-discovery"
}

// NewDiscoveryService returns the discovery service associated to the given cluster
// It is used by nodes to talk to each other.
func NewDiscoveryService(namespace string, stackName string, clusterID string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      DiscoveryServiceName(stackName),
			Labels:    ClusterIDLabels(clusterID),
		},
		Spec: corev1.ServiceSpec{
			Selector: ClusterIDLabels(clusterID),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     TransportPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeClusterIP,
			// Nodes need to discover themselves before the pod is considered ready,
			// otherwise minimum master nodes would never be reached
			PublishNotReadyAddresses: true,
		},
	}
}

// PublicServiceName returns the name for the public service
// associated to this cluster
func PublicServiceName(stackName string) string {
	return stackName + "-es-public"
}

// PublicServiceURL returns the URL used to reach Elasticsearch public endpoint
func PublicServiceURL(stackName string) string {
	return fmt.Sprintf("%s:%d", PublicServiceName(stackName), HTTPPort)
}

// NewPublicService returns the public service associated to the given cluster
// It is used by users to perform requests against one of the cluster nodes.
func NewPublicService(namespace string, stackName string, clusterID string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      PublicServiceName(stackName),
			Labels:    ClusterIDLabels(clusterID),
		},
		Spec: corev1.ServiceSpec{
			Selector: ClusterIDLabels(clusterID),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			// For now, expose the service as node port to ease development
			// TODO: proper ingress forwarding
			Type:                  corev1.ServiceTypeNodePort,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
		},
	}
}
