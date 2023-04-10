// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/apimachinery/pkg/util/intstr"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

const (
	ConfigVolumeName = "config"
	ConfigMountPath  = "/usr/share/logstash/config"

	LogstashConfigVolumeName = "logstash"
	LogstashConfigFileName   = "logstash.yml"

	PipelineVolumeName = "pipeline"
	PipelineFileName   = "pipelines.yml"

	// ConfigHashAnnotationName is an annotation used to store the Logstash config hash.
	ConfigHashAnnotationName = "logstash.k8s.elastic.co/config-hash"

	// VersionLabelName is a label used to track the version of a Logstash Pod.
	VersionLabelName = "logstash.k8s.elastic.co/version"

	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config-local"


	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName      = "elastic-internal-logstash-config"
	InternalConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config"
	InternalPipelineVolumeName      = "elastic-internal-logstash-pipeline"
	InternalPipelineVolumeMountPath = "/mnt/elastic-internal/logstash-pipeline"

)

var (
	DefaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			corev1.ResourceCPU:    resource.MustParse("2000m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			corev1.ResourceCPU:    resource.MustParse("1000m"),
		},
	}
)

var (
	// ConfigSharedVolume contains the Logstash config/ directory, it contains the contents of config from the docker container
	ConfigSharedVolume = volume.SharedVolume{
		VolumeName:             ConfigVolumeName,
		InitContainerMountPath: InitContainerConfigVolumeMountPath,
		ContainerMountPath:     ConfigMountPath,
	}
)

// ConfigVolume returns a SecretVolume to hold the Logstash config of the given Logstash resource.
func ConfigVolume(ls logstashv1alpha1.Logstash) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		logstashv1alpha1.ConfigSecretName(ls.Name),
		InternalConfigVolumeName,
		InternalConfigVolumeMountPath,
	)
}

// PipelineVolume returns a SecretVolume to hold the Logstash config of the given Logstash resource.
func PipelineVolume(ls logstashv1alpha1.Logstash) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		logstashv1alpha1.PipelineSecretName(ls.Name),
		InternalPipelineVolumeName,
		InternalPipelineVolumeMountPath,
	)
}


func buildPodTemplate(params Params, configHash hash.Hash32) corev1.PodTemplateSpec {
	defer tracing.Span(&params.Context)()
	spec := &params.Logstash.Spec
	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), logstashv1alpha1.LogstashContainerName)
	vols := []volume.VolumeLike{ConfigSharedVolume, ConfigVolume(params.Logstash), PipelineVolume(params.Logstash)}

	labels := maps.Merge(params.Logstash.GetIdentityLabels(), map[string]string{
		VersionLabelName: spec.Version})

	annotations := map[string]string{
		ConfigHashAnnotationName: fmt.Sprint(configHash.Sum32()),
	}

	ports := getDefaultContainerPorts()

	builder = builder.
		WithResources(DefaultResources).
		WithLabels(labels).
		WithAnnotations(annotations).
		WithDockerImage(spec.Image, container.ImageRepository(container.LogstashImage, spec.Version)).
		WithAutomountServiceAccountToken().
		WithPorts(ports).
		WithReadinessProbe(readinessProbe(false)).
		WithVolumeLikes(vols...).
		WithInitContainers(initConfigContainer(params.Logstash))

	builder, err := stackmon.WithMonitoring(params.Context, params.Client, builder, params.Logstash)
	if err != nil {
		return corev1.PodTemplateSpec{}
	}

	//  TODO integrate with api.ssl.enabled
	//  if params.Logstash.Spec.HTTP.TLS.Enabled() {
	//	httpVol := certificates.HTTPCertSecretVolume(logstashv1alpha1.Namer, params.Logstash.Name)
	//	builder.
	//		WithVolumes(httpVol.Volume()).
	//		WithVolumeMounts(httpVol.VolumeMount())
	//  }

	return builder.PodTemplate
}

func getDefaultContainerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{Name: "http", ContainerPort: int32(network.HTTPPort), Protocol: corev1.ProtocolTCP},
	}
}

// readinessProbe is the readiness probe for the Logstash container
func readinessProbe(useTLS bool) corev1.Probe {
	scheme := corev1.URISchemeHTTP
	if useTLS {
		scheme = corev1.URISchemeHTTPS
	}
	return corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(network.HTTPPort),
				Path:   "/",
				Scheme: scheme,
			},
		},
	}
}
