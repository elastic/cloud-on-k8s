// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// DefaultImageRepository is the default image name without a tag
	DefaultImageRepository string = "docker.elastic.co/elasticsearch/elasticsearch"

	// DefaultTerminationGracePeriodSeconds is the termination grace period for the Elasticsearch containers
	DefaultTerminationGracePeriodSeconds int64 = 120
)

var (
	// DefaultContainerPorts are the default Elasticsearch port mappings
	DefaultContainerPorts = []corev1.ContainerPort{
		{Name: "http", ContainerPort: network.HTTPPort, Protocol: corev1.ProtocolTCP},
		{Name: "transport", ContainerPort: network.TransportPort, Protocol: corev1.ProtocolTCP},
		{Name: "process-manager", ContainerPort: processmanager.DefaultPort, Protocol: corev1.ProtocolTCP},
	}
)

// PodWithConfig contains a pod and its configuration
type PodWithConfig struct {
	Pod    corev1.Pod
	Config *settings.CanonicalConfig
}

// PodsWithConfig is simply a list of PodWithConfig
type PodsWithConfig []PodWithConfig

// Pods is a helper method to retrieve pods only (no configuration)
func (p PodsWithConfig) Pods() []corev1.Pod {
	pods := make([]corev1.Pod, len(p))
	for i, withConfig := range p {
		pods[i] = withConfig.Pod
	}
	return pods
}

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
	// Config is the user provided Elasticsearch configuration.
	Config v1alpha1.Config

	// Affinity is the pod's scheduling constraints
	Affinity *corev1.Affinity

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	// Defaults to true if not specified. To be disabled, it must be explicitly set to false.
	SetVMMaxMapCount *bool

	// Resources is the memory/cpu resources the pod wants
	Resources corev1.ResourceRequirements

	// ESConfigVolume is the secret volume that contains elasticsearch.yml configuration
	ESConfigVolume volume.SecretVolume
	// UsersSecretVolume is the volume that contains x-pack configuration (users, users_roles)
	UsersSecretVolume volume.SecretVolume
	// ConfigMapVolume is a volume containing a config map with configuration files
	ConfigMapVolume volume.ConfigMapVolume
	// ClusterSecretsRef is a reference to a secret containing generic secrets shared between pods in the cluster.
	ClusterSecretsRef types.NamespacedName
	// ProbeUser is the user that should be used for the readiness probes.
	ProbeUser client.UserAuth
	// ReloadCredsUser is the user that should be used for reloading the credentials.
	ReloadCredsUser client.UserAuth
	// UnicastHostsVolume contains a file with the seed hosts.
	UnicastHostsVolume volume.ConfigMapVolume
}

// PodSpecContext contains a PodSpec and some additional context pertaining to its creation.
type PodSpecContext struct {
	PodSpec  corev1.PodSpec
	NodeSpec v1alpha1.NodeSpec
	Config   *settings.CanonicalConfig
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
