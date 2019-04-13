// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package services

import (
	"strconv"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	globalServiceSuffix = ".svc.cluster.local"
)

// DiscoveryServiceName returns the name for the discovery service
// associated to this cluster
func DiscoveryServiceName(esName string) string {
	return name.DiscoveryService(esName)
}

// NewDiscoveryService returns the discovery service associated to the given cluster
// It is used by nodes to talk to each other.
func NewDiscoveryService(es v1alpha1.Elasticsearch) *corev1.Service {
	nsn := k8s.ExtractNamespacedName(&es)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      DiscoveryServiceName(es.Name),
			Labels:    label.NewLabels(nsn),
		},
		Spec: corev1.ServiceSpec{
			Selector: label.NewLabels(nsn),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     network.TransportPort,
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

// ExternalServiceName returns the name for the external service
// associated to this cluster
func ExternalServiceName(esName string) string {
	return name.Service(esName)
}

// ExternalServiceURL returns the URL used to reach Elasticsearch's external endpoint
func ExternalServiceURL(es v1alpha1.Elasticsearch) string {
	return stringsutil.Concat(network.ProtocolForCluster(es), "://", ExternalServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.HTTPPort))
}

// ExternalDiscoveryServiceHostname returns the hostname used to reach Elasticsearch's discovery endpoint.
func ExternalDiscoveryServiceHostname(es types.NamespacedName) string {
	return stringsutil.Concat(DiscoveryServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.TransportPort))
}

// NewExternalService returns the external service associated to the given cluster
// It is used by users to perform requests against one of the cluster nodes.
func NewExternalService(es v1alpha1.Elasticsearch) *corev1.Service {
	nsn := k8s.ExtractNamespacedName(&es)
	var svc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      ExternalServiceName(es.Name),
			Labels:    label.NewLabels(nsn),
		},
		Spec: corev1.ServiceSpec{
			Selector: label.NewLabels(nsn),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name:     network.ProtocolForCluster(es),
					Protocol: corev1.ProtocolTCP,
					Port:     network.HTTPPort,
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
func IsServiceReady(c k8s.Client, service corev1.Service) (bool, error) {
	endpoints := corev1.Endpoints{}
	namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}

	if err := c.Get(namespacedName, &endpoints); err != nil {
		return false, err
	}
	for _, subs := range endpoints.Subsets {
		if len(subs.Addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// GetExternalService returns the external service associated to the given Elasticsearch cluster.
func GetExternalService(c k8s.Client, es v1alpha1.Elasticsearch) (corev1.Service, error) {
	var svc corev1.Service

	namespacedName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      ExternalServiceName(es.Name),
	}

	if err := c.Get(namespacedName, &svc); err != nil {
		return corev1.Service{}, err
	}

	return svc, nil
}
