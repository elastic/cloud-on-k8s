// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/securesettings"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
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

// DefaultAffinity returns the default affinity for pods in a cluster.
func DefaultAffinity(esName string) *corev1.Affinity {
	return &corev1.Affinity{
		// prefer to avoid two pods in the same cluster being co-located on a single node
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: "kubernetes.io/hostname",
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName: esName,
							},
						},
					},
				},
			},
		},
	}
}

// PodWithConfig contains a pod and its configuration
type PodWithConfig struct {
	Pod    corev1.Pod
	Config settings.CanonicalConfig
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
	// Elasticsearch is the Elasticsearch cluster specification.
	Elasticsearch v1alpha1.Elasticsearch

	// DiscoveryZenMinimumMasterNodes is the setting for minimum master node in Zen Discovery
	DiscoveryZenMinimumMasterNodes int

	// NodeSpec is the user-provided spec to apply on the target pod
	NodeSpec v1alpha1.NodeSpec

	// ESConfigVolume is the secret volume that contains elasticsearch.yml configuration
	ESConfigVolume volume.SecretVolume
	// UsersSecretVolume is the volume that contains x-pack configuration (users, users_roles)
	UsersSecretVolume volume.SecretVolume
	// ProbeUser is the user that should be used for the readiness probes.
	ProbeUser client.UserAuth
	// KeystoreUser is the user that should be used for reloading the credentials.
	KeystoreUser client.UserAuth
	// UnicastHostsVolume contains a file with the seed hosts.
	UnicastHostsVolume volume.ConfigMapVolume
	// SecureSettings contains k8s resources to load secure settings in the keystore
	SecureSettings securesettings.SecureSettings
}

// PodSpecContext contains a pod template and some additional context pertaining to its creation.
type PodSpecContext struct {
	PodTemplate corev1.PodTemplateSpec
	NodeSpec    v1alpha1.NodeSpec
	Config      settings.CanonicalConfig
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
