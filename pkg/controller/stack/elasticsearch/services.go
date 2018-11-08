package elasticsearch

import (
	"strconv"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiscoveryServiceName returns the name for the discovery service
// associated to this cluster
func DiscoveryServiceName(stackName string) string {
	return common.Concat(stackName, "-es-discovery")
}

// NewDiscoveryService returns the discovery service associated to the given cluster
// It is used by nodes to talk to each other.
func NewDiscoveryService(s deploymentsv1alpha1.Stack) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      DiscoveryServiceName(s.Name),
			Labels:    NewLabels(s, false),
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabels(s, false),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     TransportPort,
				},
			},
			ClusterIP:       "None",
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
	return common.Concat(stackName, "-es-public")
}

// PublicServiceURL returns the URL used to reach Elasticsearch public endpoint
func PublicServiceURL(stackName string) string {
	scheme := "http"
	return common.Concat(scheme, "://", PublicServiceName(stackName), ":", strconv.Itoa(HTTPPort))
}

// NewPublicService returns the public service associated to the given cluster
// It is used by users to perform requests against one of the cluster nodes.
func NewPublicService(s deploymentsv1alpha1.Stack) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      PublicServiceName(s.Name),
			Labels:    NewLabels(s, false),
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabels(s, false),
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
