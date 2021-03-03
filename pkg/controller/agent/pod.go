// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"fmt"
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	CAFileName = "ca.crt"

	ContainerName = "agent"

	ConfigVolumeName = "config"
	ConfigMountPath  = "/etc/agent.yml"
	ConfigFileName   = "agent.yml"

	DataVolumeName            = "agent-data"
	DataMountHostPathTemplate = "/var/lib/%s/%s/agent-data"
	DataMountPath             = "/usr/share/data"

	// ConfigChecksumLabel is a label used to store Agent config checksum.
	ConfigChecksumLabel = "agent.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Agent Pod.
	VersionLabelName = "agent.k8s.elastic.co/version"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}
)

func buildPodTemplate(params Params, configHash hash.Hash) corev1.PodTemplateSpec {
	defer tracing.Span(&params.Context)()

	podTemplate := params.GetPodTemplate()

	spec := &params.Agent.Spec

	labels := maps.Merge(NewLabels(params.Agent), map[string]string{
		ConfigChecksumLabel: fmt.Sprintf("%x", configHash.Sum(nil)),
		VersionLabelName:    spec.Version})

	dataVolume := createDataVolume(params)
	vols := []volume.VolumeLike{
		volume.NewSecretVolume(
			ConfigSecretName(params.Agent.Name),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0440),
		dataVolume,
	}

	for i, association := range params.Agent.GetAssociations() {
		if !association.AssociationConf().CAIsConfigured() {
			continue
		}
		caSecretName := association.AssociationConf().GetCASecretName()
		caVolume := volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("%s-certs-%d", association.AssociationType(), i),
			certificatesDir(association),
		)
		vols = append(vols, caVolume)
	}

	volumes := make([]corev1.Volume, 0, len(vols))
	volumeMounts := make([]corev1.VolumeMount, 0, len(vols))
	var initContainers []corev1.Container

	for _, v := range vols {
		volumes = append(volumes, v.Volume())
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, ContainerName).
		WithLabels(labels).
		WithResources(defaultResources).
		WithDockerImage(spec.Image, container.ImageRepository(container.AgentImage, spec.Version)).
		WithArgs("-e", "-c", ConfigMountPath).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initContainers...).
		WithInitContainerDefaults()

	return builder.PodTemplate
}

func createDataVolume(params Params) volume.VolumeLike {
	dataMountHostPath := fmt.Sprintf(DataMountHostPathTemplate, params.Agent.Namespace, params.Agent.Name)

	return volume.NewHostVolume(
		DataVolumeName,
		dataMountHostPath,
		DataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)
}

func certificatesDir(association commonv1.Association) string {
	ref := association.AssociationRef()
	return fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		association.AssociationType(),
		ref.Namespace,
		ref.Name,
	)
}
