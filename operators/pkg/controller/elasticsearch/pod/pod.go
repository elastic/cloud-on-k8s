// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// HTTPPort used by Elasticsearch for the REST API
	HTTPPort = 9200
	// TransportPort used by Elasticsearch for the Transport protocol
	TransportPort = 9300
	// TransportClientPort used by Elasticsearch for the Transport protocol for client-only connections
	TransportClientPort = 9400

	// DefaultImageRepository is the default image name without a tag
	DefaultImageRepository string = "docker.elastic.co/elasticsearch/elasticsearch"

	// DefaultTerminationGracePeriodSeconds is the termination grace period for the Elasticsearch containers
	DefaultTerminationGracePeriodSeconds int64 = 120

	// DefaultContainerName is the name of the elasticsearch container
	DefaultContainerName = "elasticsearch"
)

var (
	// DefaultContainerPorts are the default Elasticsearch port mappings
	DefaultContainerPorts = []corev1.ContainerPort{
		{Name: "http", ContainerPort: HTTPPort, Protocol: corev1.ProtocolTCP},
		{Name: "transport", ContainerPort: TransportPort, Protocol: corev1.ProtocolTCP},
		{Name: "client", ContainerPort: TransportClientPort, Protocol: corev1.ProtocolTCP},
	}
)

// NewPodSpecParams is used to build resources associated with an Elasticsearch Cluster
type NewPodSpecParams struct {
	// Version is the Elasticsearch version
	Version string
	// CustomImageName is the custom image used, leave empty for the default
	CustomImageName string
	// ClusterName is the name of the Elasticsearch cluster
	ClusterName string
	// DiscoveryServiceName is the name of the Service that should be used for discovery.
	DiscoveryServiceName string
	// DiscoveryZenMinimumMasterNodes is the setting for minimum master node in Zen Discovery
	DiscoveryZenMinimumMasterNodes int
	// NodeTypes defines the type (master/data/ingest) associated to the ES node
	NodeTypes v1alpha1.NodeTypesSpec

	// Affinity is the pod's scheduling constraints
	Affinity *corev1.Affinity

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	SetVMMaxMapCount bool

	// Resources is the memory/cpu resources the pod wants
	Resources commonv1alpha1.ResourcesSpec

	// UsersSecretVolume is the volume that contains x-pack configuration (users, users_roles)
	UsersSecretVolume volume.SecretVolume
	// ConfigMapVolume is a volume containing a config map with configuration files
	ConfigMapVolume volume.ConfigMapVolume
	// ExtraFilesRef is a reference to a secret containing generic extra resources for the pod.
	ExtraFilesRef types.NamespacedName
	// KeystoreSecretRef is configuration for the Elasticsearch key store setup
	KeystoreSecretRef types.NamespacedName
	// ProbeUser is the user that should be used for the readiness probes.
	ProbeUser client.User
	// ReloadCredsUser is the user that should be used for reloading the credentials.
	ReloadCredsUser client.User
}

// PodSpecContext contains a PodSpec and some additional context pertaining to its creation.
type PodSpecContext struct {
	PodSpec      corev1.PodSpec
	TopologySpec v1alpha1.ElasticsearchTopologySpec
}

// PodListToNames returns a list of pod names from the list of pods.
func PodListToNames(pods []corev1.Pod) []string {
	names := make([]string, len(pods))
	for i, pod := range pods {
		names[i] = pod.Name
	}
	return names
}

// PodMapToNames returns a list of pod names from a map of pod names to pods
func PodMapToNames(pods map[string]corev1.Pod) []string {
	names := make([]string, 0, len(pods))
	for podName := range pods {
		names = append(names, podName)
	}
	return names
}
