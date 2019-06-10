// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// DefaultVolumeClaimsTemplates is the default volume claim templates for Elasticsearch pods
	DefaultVolumeClaimsTemplates = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "elasticsearch-data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
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
	// Version is the Elasticsearch version
	Version string
	// CustomImageName is the custom image used, leave empty for the default
	CustomImageName string
	// ClusterName is the name of the Elasticsearch cluster
	ClusterName string
	// DiscoveryZenMinimumMasterNodes is the setting for minimum master node in Zen Discovery
	DiscoveryZenMinimumMasterNodes int

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	// Defaults to true if not specified. To be disabled, it must be explicitly set to false.
	SetVMMaxMapCount *bool

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
}

// PodSpecContext contains a PodSpec and some additional context pertaining to its creation.
type PodSpecContext struct {
	PodSpec  corev1.PodSpec
	NodeSpec v1alpha1.NodeSpec
	Config   settings.CanonicalConfig
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
