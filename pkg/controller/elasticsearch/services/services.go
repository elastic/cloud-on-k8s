// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package services

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

const (
	globalServiceSuffix = ".svc"
)

// TransportServiceName returns the name for the transport service associated to this cluster
func TransportServiceName(esName string) string {
	return esv1.TransportService(esName)
}

// NewTransportService returns the transport service associated with the given cluster.
// It is used by Elasticsearch nodes to talk to remote cluster nodes.
func NewTransportService(es esv1.Elasticsearch) *corev1.Service {
	nsn := k8s.ExtractNamespacedName(&es)
	svc := corev1.Service{
		ObjectMeta: es.Spec.Transport.Service.ObjectMeta,
		Spec:       es.Spec.Transport.Service.Spec,
	}

	svc.ObjectMeta.Namespace = es.Namespace
	svc.ObjectMeta.Name = TransportServiceName(es.Name)
	// Nodes need to discover themselves before the pod is considered ready,
	// otherwise minimum master nodes would never be reached
	svc.Spec.PublishNotReadyAddresses = true
	if svc.Spec.Type == "" {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		// We set ClusterIP to None in order to let the ES nodes discover all other node IPs at once.
		svc.Spec.ClusterIP = "None"
	}
	labels := label.NewLabels(nsn)
	ports := []corev1.ServicePort{
		{
			Name:     "tls-transport", // prefix with protocol for Istio compatibility
			Protocol: corev1.ProtocolTCP,
			Port:     network.TransportPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}

// ExternalServiceName returns the name for the external service
// associated to this cluster
func ExternalServiceName(esName string) string {
	return esv1.HTTPService(esName)
}

// InternalServiceName returns the name for the internal service
// associated to this cluster, managed by the operator exclusively.
func InternalServiceName(esName string) string {
	return esv1.InternalHTTPService(esName)
}

// ExternalTransportServiceHost returns the hostname and the port used to reach Elasticsearch's transport endpoint.
func ExternalTransportServiceHost(es types.NamespacedName) string {
	return stringsutil.Concat(TransportServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.TransportPort))
}

// ExternalServiceURL returns the URL used to reach Elasticsearch's external endpoint.
func ExternalServiceURL(es esv1.Elasticsearch) string {
	return stringsutil.Concat(es.Spec.HTTP.Protocol(), "://", ExternalServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.HTTPPort))
}

// InternalServiceURL returns the URL used to reach Elasticsearch's internally managed service
func InternalServiceURL(es esv1.Elasticsearch) string {
	return stringsutil.Concat(es.Spec.HTTP.Protocol(), "://", InternalServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.HTTPPort))
}

// NewExternalService returns the external service associated to the given cluster.
// It is used by users to perform requests against one of the cluster nodes.
func NewExternalService(es esv1.Elasticsearch) *corev1.Service {
	nsn := k8s.ExtractNamespacedName(&es)

	svc := corev1.Service{
		ObjectMeta: es.Spec.HTTP.Service.ObjectMeta,
		Spec:       es.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = es.Namespace
	svc.ObjectMeta.Name = ExternalServiceName(es.Name)

	labels := label.NewLabels(nsn)
	ports := []corev1.ServicePort{
		{
			Name:     es.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}

// NewInternalService returns the internal service associated to the given cluster.
// It is used by the operator to perform requests against the Elasticsearch cluster nodes,
// and does not inherit the spec defined within the Elasticsearch custom resource,
// to remove the possibility of the user misconfiguring access to the ES cluster.
func NewInternalService(es esv1.Elasticsearch) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InternalServiceName(es.Name),
			Namespace: es.Namespace,
			Labels:    label.NewLabels(k8s.ExtractNamespacedName(&es)),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:     es.Spec.HTTP.Protocol(),
					Protocol: corev1.ProtocolTCP,
					Port:     network.HTTPPort,
				},
			},
			Selector:                 label.NewLabels(k8s.ExtractNamespacedName(&es)),
			PublishNotReadyAddresses: false,
		},
	}
}

// IsServiceReady checks if a service has one or more ready endpoints.
func IsServiceReady(c k8s.Client, service corev1.Service) (bool, error) {
	endpoints := corev1.Endpoints{}
	namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}

	if err := c.Get(context.Background(), namespacedName, &endpoints); err != nil {
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
func GetExternalService(c k8s.Client, es esv1.Elasticsearch) (corev1.Service, error) {
	return getServiceByName(c, es, ExternalServiceName(es.Name))
}

// GetInternalService returns the internally managed service associated to the given Elasticsearch cluster.
func GetInternalService(c k8s.Client, es esv1.Elasticsearch) (corev1.Service, error) {
	return getServiceByName(c, es, InternalServiceName(es.Name))
}

func getServiceByName(c k8s.Client, es esv1.Elasticsearch, serviceName string) (corev1.Service, error) {
	var svc corev1.Service

	namespacedName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      serviceName,
	}

	if err := c.Get(context.Background(), namespacedName, &svc); err != nil {
		return corev1.Service{}, err
	}

	return svc, nil
}

// ElasticsearchURL calculates the base url for Elasticsearch, taking into account the currently running pods.
// If there is an HTTP scheme mismatch between spec and pods we switch to requesting individual pods directly
// otherwise this delegates to ExternalServiceURL.
func ElasticsearchURL(es esv1.Elasticsearch, pods []corev1.Pod) string {
	var schemeChange bool
	for _, p := range pods {
		scheme, exists := p.Labels[label.HTTPSchemeLabelName]
		if exists && scheme != es.Spec.HTTP.Protocol() {
			// scheme in existing pods does not match scheme in spec, user toggled HTTP(S)
			schemeChange = true
		}
	}
	if schemeChange {
		// switch to sending requests directly to a random pod instead of going through the service
		randomPod := pods[rand.Intn(len(pods))] //nolint:gosec
		if podURL := ElasticsearchPodURL(randomPod); podURL != "" {
			return podURL
		}
	}
	return InternalServiceURL(es)
}

// ElasticsearchPodURL calculates the URL for the given Pod based on the Pods metadata.
func ElasticsearchPodURL(pod corev1.Pod) string {
	scheme, hasSchemeLabel := pod.Labels[label.HTTPSchemeLabelName]
	sset, hasSsetLabel := pod.Labels[label.StatefulSetNameLabelName]
	if hasSsetLabel && hasSchemeLabel {
		return fmt.Sprintf("%s://%s.%s.%s:%d", scheme, pod.Name, sset, pod.Namespace, network.HTTPPort)
	}
	return ""
}
