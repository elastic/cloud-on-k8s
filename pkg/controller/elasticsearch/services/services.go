// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package services

import (
	"fmt"
	"math/rand"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/stringsutil"
)

const (
	globalServiceSuffix = ".svc"

	RemoteClusterServicePortName = "rcs"
)

// TransportServiceName returns the name for the transport service associated to this cluster
func TransportServiceName(esName string) string {
	return esv1.TransportService(esName)
}

// NewTransportService returns the transport service associated with the given cluster.
// It is used by Elasticsearch nodes to talk to remote cluster nodes.
func NewTransportService(es esv1.Elasticsearch, meta metadata.Metadata) *corev1.Service {
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
	selector := label.NewLabels(nsn)
	ports := []corev1.ServicePort{
		{
			Name:     "tls-transport", // prefix with protocol for Istio compatibility
			Protocol: corev1.ProtocolTCP,
			Port:     network.TransportPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, meta, selector, ports)
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

// RemoteClusterServiceName returns the name for the remote cluster service used when the cluster is expected to be accessed
// using the remote cluster server. Managed by the operator exclusively.
func RemoteClusterServiceName(esName string) string {
	return esv1.RemoteClusterService(esName)
}

// ExternalTransportServiceHost returns the hostname and the port used to reach Elasticsearch's transport endpoint.
func ExternalTransportServiceHost(es types.NamespacedName) string {
	return stringsutil.Concat(TransportServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.TransportPort))
}

// RemoteClusterServerServiceHost returns the hostname and the port used to reach Elasticsearch's remote cluster server endpoint.
func RemoteClusterServerServiceHost(es types.NamespacedName) string {
	return stringsutil.Concat(RemoteClusterServiceName(es.Name), ".", es.Namespace, globalServiceSuffix, ":", strconv.Itoa(network.RemoteClusterPort))
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
func NewExternalService(es esv1.Elasticsearch, meta metadata.Metadata) *corev1.Service {
	nsn := k8s.ExtractNamespacedName(&es)

	svc := corev1.Service{
		ObjectMeta: es.Spec.HTTP.Service.ObjectMeta,
		Spec:       es.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = es.Namespace
	svc.ObjectMeta.Name = ExternalServiceName(es.Name)

	// defaults to ClusterIP if not set
	if svc.Spec.Type == "" {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}
	selector := label.NewLabels(nsn)
	ports := []corev1.ServicePort{
		{
			Name:     es.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, meta, selector, ports)
}

// NewInternalService returns the internal service associated to the given cluster.
// It is used by the operator to perform requests against the Elasticsearch cluster nodes,
// and does not inherit the spec defined within the Elasticsearch custom resource,
// to remove the possibility of the user misconfiguring access to the ES cluster.
func NewInternalService(es esv1.Elasticsearch, meta metadata.Metadata) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        InternalServiceName(es.Name),
			Namespace:   es.Namespace,
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
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

// NewRemoteClusterService returns the service associated to the remote cluster service for the given cluster.
func NewRemoteClusterService(es esv1.Elasticsearch, meta metadata.Metadata) *corev1.Service {
	nsn := k8s.ExtractNamespacedName(&es)
	svc := corev1.Service{
		ObjectMeta: es.Spec.RemoteClusterServer.Service.ObjectMeta,
		Spec:       es.Spec.RemoteClusterServer.Service.Spec,
	}

	svc.ObjectMeta.Namespace = es.Namespace
	svc.ObjectMeta.Name = RemoteClusterServiceName(es.Name)
	// Allow connections to pods that are not yet ready
	svc.Spec.PublishNotReadyAddresses = true
	if svc.Spec.Type == "" {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		// ClusterIP None creates a headless service, allowing direct access to all pods for remote cluster connections
		svc.Spec.ClusterIP = "None"
	}
	selector := label.NewLabels(nsn)
	ports := []corev1.ServicePort{
		{
			Name:     RemoteClusterServicePortName,
			Protocol: corev1.ProtocolTCP,
			Port:     network.RemoteClusterPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, meta, selector, ports)
}

type urlProvider struct {
	pods   func() ([]corev1.Pod, error)
	svcURL string
}

// URL implements client.URLProvider.
func (u *urlProvider) URL() (string, error) {
	var ready, running []corev1.Pod
	pods, err := u.pods()
	if err != nil {
		return "", err
	}
	for _, p := range pods {
		if k8s.IsPodReady(p) {
			ready = append(ready, p)
		}
		if k8s.IsPodRunning(p) {
			running = append(running, p)
		}
	}
	switch {
	case len(ready) > 0:
		return randomESPodURL(ready), nil
	case len(running) > 0:
		return randomESPodURL(running), nil
	default:
		return u.svcURL, nil
	}
}

// Equals implements client.URLProvider.
func (u *urlProvider) Equals(other client.URLProvider) bool {
	otherImpl, ok := other.(*urlProvider)
	if !ok {
		return false
	}
	return u.svcURL == otherImpl.svcURL
}

// HasEndpoints implements client.URLProvider.
func (u *urlProvider) HasEndpoints() bool {
	pods, err := u.pods()
	return err == nil && len(k8s.RunningPods(pods)) > 0
}

// NewElasticsearchURLProvider returns a client.URLProvider that dynamically tries to find Pod URLs among the
// currently running Pods. Preferring ready Pods over running ones.
func NewElasticsearchURLProvider(es esv1.Elasticsearch, client k8s.Client) client.URLProvider {
	return &urlProvider{
		pods: func() ([]corev1.Pod, error) {
			return k8s.PodsMatchingLabels(client, es.Namespace, label.NewLabelSelectorForElasticsearch(es))
		},
		svcURL: InternalServiceURL(es),
	}
}

func randomESPodURL(pods []corev1.Pod) string {
	randomPod := pods[rand.Intn(len(pods))] //nolint:gosec
	return ElasticsearchPodURL(randomPod)
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
