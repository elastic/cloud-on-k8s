// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"fmt"
	"hash"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	CAFileName = "ca.crt"

	ConfigVolumeName = "config"
	ConfigMountPath  = "/etc/beat.yml"
	ConfigFileName   = "beat.yml"

	DataVolumeName        = "beat-data"
	DataMountPathTemplate = "/var/lib/%s/%s/%s-data"
	DataPathTemplate      = "/usr/share/%s/data"

	// ConfigChecksumLabel is a label used to store a Beat config checksum.
	ConfigChecksumLabel = "beat.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Beat Pod.
	VersionLabelName = "beat.k8s.elastic.co/version"
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

func certificatesDir(association commonv1.Association) string {
	return fmt.Sprintf("/mnt/elastic-internal/%s-certs", association.AssociationType())
}

// initContainerParameters generates parameters specific to Beats for an init container that will load the secure
// settings into a keystore
func initContainerParameters(typ string) keystore.InitContainerParameters {
	return keystore.InitContainerParameters{
		KeystoreCreateCommand:         fmt.Sprintf("%s keystore create --force", typ),
		KeystoreAddCommand:            fmt.Sprintf(`cat "$filename" | %s keystore add "$key" --stdin --force`, typ),
		SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
		KeystoreVolumePath:            fmt.Sprintf(DataPathTemplate, typ),
		Resources:                     defaultResources,
	}
}

func buildPodTemplate(
	params DriverParams,
	defaultImage container.Image,
	keystoreResources *keystore.Resources,
	configHash hash.Hash,
) corev1.PodTemplateSpec {
	podTemplate := params.GetPodTemplate()

	spec := &params.Beat.Spec

	labels := maps.Merge(NewLabels(params.Beat), map[string]string{
		ConfigChecksumLabel: fmt.Sprintf("%x", configHash.Sum(nil)),
		VersionLabelName:    spec.Version})

	dataVolume := createDataVolume(params)
	vols := []volume.VolumeLike{
		volume.NewSecretVolume(
			ConfigSecretName(spec.Type, params.Beat.Name),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0600),
		dataVolume,
	}

	for _, association := range params.Beat.GetAssociations() {
		if !association.AssociationConf().CAIsConfigured() {
			continue
		}
		caSecretName := association.AssociationConf().GetCASecretName()
		caVolume := volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("%s-certs", association.AssociationType()),
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

	if keystoreResources != nil {
		_, _ = configHash.Write([]byte(keystoreResources.Version))
		volumes = append(volumes, keystoreResources.Volume)
		initContainers = append(initContainers, keystoreResources.InitContainer)
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, spec.Type).
		WithLabels(labels).
		WithResources(defaultResources).
		WithDockerImage(spec.Image, container.ImageRepository(defaultImage, spec.Version)).
		WithArgs("-e", "-c", ConfigMountPath).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initContainers...).
		WithInitContainerDefaults()

	return builder.PodTemplate
}

func createDataVolume(dp DriverParams) volume.VolumeLike {
	dataMountPath := fmt.Sprintf(DataPathTemplate, dp.Beat.Spec.Type)
	hostDataPath := fmt.Sprintf(DataMountPathTemplate, dp.Beat.Namespace, dp.Beat.Name, dp.Beat.Spec.Type)

	return volume.NewHostVolume(
		DataVolumeName,
		hostDataPath,
		dataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)
}
