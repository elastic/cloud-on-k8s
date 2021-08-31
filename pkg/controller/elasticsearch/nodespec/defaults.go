// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
)

const (
	// DefaultTerminationGracePeriodSeconds is the termination grace period for the Elasticsearch containers
	DefaultTerminationGracePeriodSeconds int64 = 180
)

var (
	DefaultMemoryLimits = resource.MustParse("2Gi")
	// DefaultResources for the Elasticsearch container. The JVM default heap size is 1Gi, so we
	// request at least 2Gi for the container to make sure ES can work properly.
	// Not applying this minimum default would make ES randomly crash (OOM) on small machines.
	// Similarly, we apply a default memory limit of 2Gi, to ensure the Pod isn't the first one to get evicted.
	// No CPU requirement is set by default.
	DefaultResources = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
	}

	// DefaultAnnotations are the default annotations for the Elasticsearch pods
	DefaultAnnotations = map[string]string{
		annotation.FilebeatModuleAnnotation: "elasticsearch",
	}
)

// DefaultEnvVars are environment variables injected into Elasticsearch pods.
func DefaultEnvVars(httpCfg commonv1.HTTPConfig, headlessServiceName string) []corev1.EnvVar {
	return defaults.ExtendPodDownwardEnvVars(
		[]corev1.EnvVar{
			{Name: settings.EnvProbePasswordPath, Value: path.Join(esvolume.ProbeUserSecretMountPath, user.ProbeUserName)},
			{Name: settings.EnvProbeUsername, Value: user.ProbeUserName},
			{Name: settings.EnvReadinessProbeProtocol, Value: httpCfg.Protocol()},
			{Name: settings.HeadlessServiceName, Value: headlessServiceName},

			// Disable curl/libnss use of sqlite caching to avoid triggering an issue in linux/kubernetes
			// where the kernel's dentry cache grows by 5mb every time curl is invoked. This cache usage
			// is charged against the pod which created it. In our case, the elasticsearch nodes trigger
			// this problem with the readinessProbe invoking curl.
			//
			// In production testing, no negative impact on curl's behavior is observed from this setting.
			// This setting is primarily targeted at curl invocation in the readinessProbe.
			// References:
			//   https://github.com/elastic/cloud-on-k8s/issues/1581#issuecomment-525527334
			//   https://github.com/elastic/cloud-on-k8s/issues/1635
			//   https://issuetracker.google.com/issues/140577001
			{Name: "NSS_SDB_USE_CACHE", Value: "no"},
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
