// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
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
	}

	// DefaultResources for the Elasticsearch container. The JVM default heap size is 1Gi, so we
	// request at least 2Gi for the container to make sure ES can work properly.
	// Not applying this minimum default would make ES randomly crash (OOM) on small machines.
	DefaultResources = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
)

// DefaultEnvVars are environment variables injected into Elasticsearch pods.
func DefaultEnvVars(httpCfg v1alpha1.HTTPConfig) []corev1.EnvVar {
	return append(
		defaults.PodDownwardEnvVars,
		[]corev1.EnvVar{
			{Name: settings.EnvProbePasswordFile, Value: path.Join(esvolume.ProbeUserSecretMountPath, user.InternalProbeUserName)},
			{Name: settings.EnvProbeUsername, Value: user.InternalProbeUserName},
			{Name: settings.EnvReadinessProbeProtocol, Value: httpCfg.Scheme()},
		}...,
	)
}

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
