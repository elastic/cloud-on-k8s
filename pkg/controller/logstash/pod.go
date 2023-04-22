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

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	commonassociation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
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

func buildPodTemplate(params Params, configHash hash.Hash32) (corev1.PodTemplateSpec, error) {
	defer tracing.Span(&params.Context)()
	spec := &params.Logstash.Spec
	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), logstashv1alpha1.LogstashContainerName)

	vols, err := buildVolumes(params)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	esAssociations := getEsAssociations(params)
	if err := writeEsAssocToConfigHash(params, esAssociations, configHash); err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	envs, err := buildEnv(params, esAssociations)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

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
		WithReadinessProbe(readinessProbe(params.Logstash)).
		WithVolumeLikes(vols...).
		WithInitContainers(initConfigContainer(params.Logstash)).
		WithEnv(envs...).
		WithInitContainerDefaults()

	builder, err = stackmon.WithMonitoring(params.Context, params.Client, builder, params.Logstash)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	//  TODO integrate with api.ssl.enabled
	//  if params.Logstash.Spec.HTTP.TLS.Enabled() {
	//	httpVol := certificates.HTTPCertSecretVolume(logstashv1alpha1.Namer, params.Logstash.Name)
	//	builder.
	//		WithVolumes(httpVol.Volume()).
	//		WithVolumeMounts(httpVol.VolumeMount())
	//  }

	return builder.PodTemplate, nil
}

func getDefaultContainerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{Name: "http", ContainerPort: int32(network.HTTPPort), Protocol: corev1.ProtocolTCP},
	}
}

// readinessProbe is the readiness probe for the Logstash container
func readinessProbe(logstash logstashv1alpha1.Logstash) corev1.Probe {
	var scheme = corev1.URISchemeHTTP
	var port = network.HTTPPort
	for _, service := range logstash.Spec.Services {
		if service.Name == LogstashAPIServiceName && len(service.Service.Spec.Ports) > 0 {
			port = int(service.Service.Spec.Ports[0].Port)
		}
	}
	probe := corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(port),
				Path:   "/",
				Scheme: scheme,
			},
		},
	}
	return probe
}

func getEsAssociations(params Params) []commonv1.Association {
	var esAssociations []commonv1.Association

	for _, assoc := range params.Logstash.GetAssociations() {
		if assoc.AssociationType() == commonv1.ElasticsearchAssociationType {
			esAssociations = append(esAssociations, assoc)
		}
	}
	return esAssociations
}

func writeEsAssocToConfigHash(params Params, esAssociations []commonv1.Association, configHash hash.Hash) error {
	if esAssociations == nil {
		return nil
	}

	return commonassociation.WriteAssocsToConfigHash(
		params.Client,
		esAssociations,
		configHash,
	)
}
