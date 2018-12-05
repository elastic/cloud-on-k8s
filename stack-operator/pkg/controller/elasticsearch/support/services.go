package support

import (
	"strconv"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	globalServiceSuffix = ".svc.cluster.local"
)

// DiscoveryServiceName returns the name for the discovery service
// associated to this cluster
func DiscoveryServiceName(esName string) string {
	return common.Concat(esName, "-es-discovery")
}

// NewDiscoveryService returns the discovery service associated to the given cluster
// It is used by nodes to talk to each other.
func NewDiscoveryService(es v1alpha1.ElasticsearchCluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      DiscoveryServiceName(es.Name),
			Labels:    NewLabels(es),
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabels(es),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     TransportPort,
				},
			},
			// We set ClusterIP to None in order to let the ES nodes discover all other node IPs at once.
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
func PublicServiceName(esName string) string {
	return common.Concat(esName, "-es-public")
}

// PublicServiceURL returns the URL used to reach Elasticsearch public endpoint
func PublicServiceURL(es v1alpha1.ElasticsearchCluster) string {
	return common.Concat("https://", PublicServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(HTTPPort))
}

// NewPublicService returns the public service associated to the given cluster
// It is used by users to perform requests against one of the cluster nodes.
func NewPublicService(es v1alpha1.ElasticsearchCluster) *corev1.Service {
	var svc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      PublicServiceName(es.Name),
			Labels:    NewLabels(es),
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabels(es),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            common.GetServiceType(es.Spec.Expose),
		},
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
	}
	return &svc
}
