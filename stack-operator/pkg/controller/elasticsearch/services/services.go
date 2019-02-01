package services

import (
	"context"
	"strconv"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/label"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	globalServiceSuffix = ".svc.cluster.local"
)

// DiscoveryServiceName returns the name for the discovery service
// associated to this cluster
func DiscoveryServiceName(esName string) string {
	return stringsutil.Concat(esName, "-es-discovery")
}

// NewDiscoveryService returns the discovery service associated to the given cluster
// It is used by nodes to talk to each other.
func NewDiscoveryService(es v1alpha1.ElasticsearchCluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      DiscoveryServiceName(es.Name),
			Labels:    label.NewLabels(es),
		},
		Spec: corev1.ServiceSpec{
			Selector: label.NewLabels(es),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     pod.TransportPort,
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
	return stringsutil.Concat(esName, "-es-public")
}

// PublicServiceURL returns the URL used to reach Elasticsearch public endpoint
func PublicServiceURL(es v1alpha1.ElasticsearchCluster) string {
	return stringsutil.Concat("https://", PublicServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(pod.HTTPPort))
}

// NewPublicService returns the public service associated to the given cluster
// It is used by users to perform requests against one of the cluster nodes.
func NewPublicService(es v1alpha1.ElasticsearchCluster) *corev1.Service {
	var svc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      PublicServiceName(es.Name),
			Labels:    label.NewLabels(es),
		},
		Spec: corev1.ServiceSpec{
			Selector: label.NewLabels(es),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     pod.HTTPPort,
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

// IsServiceReady checks if a service has one or more ready endpoints.
func IsServiceReady(c client.Client, service corev1.Service) (bool, error) {
	endpoints := corev1.Endpoints{}
	namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}

	if err := c.Get(context.TODO(), namespacedName, &endpoints); err != nil {
		return false, err
	}
	for _, subs := range endpoints.Subsets {
		if len(subs.Addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
}
