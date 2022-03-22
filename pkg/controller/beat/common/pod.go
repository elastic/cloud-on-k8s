// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"fmt"
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
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

	// ConfigHashAnnotationName is an annotation used to store a Beat config hash.
	ConfigHashAnnotationName = "beat.k8s.elastic.co/config-hash"

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
		SkipInitializedFlag:           true,
	}
}

func buildPodTemplate(
	params DriverParams,
	defaultImage container.Image,
	configHash hash.Hash32,
) (corev1.PodTemplateSpec, error) {
	podTemplate := params.GetPodTemplate()

	keystoreResources, err := keystore.ReconcileResources(
		params,
		&params.Beat,
		namer,
		NewLabels(params.Beat),
		initContainerParameters(params.Beat.Spec.Type),
	)
	if err != nil {
		return podTemplate, err
	}

	spec := &params.Beat.Spec
	dataVolume := createDataVolume(params)
	vols := []volume.VolumeLike{
		volume.NewSecretVolume(
			ConfigSecretName(spec.Type, params.Beat.Name),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0444),
		dataVolume,
	}

	for _, assoc := range params.Beat.GetAssociations() {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return corev1.PodTemplateSpec{}, err
		}
		if !assocConf.CAIsConfigured() {
			continue
		}
		caSecretName := assocConf.GetCASecretName()
		caVolume := volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("%s-certs", assoc.AssociationType()),
			certificatesDir(assoc),
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

	labels := maps.Merge(NewLabels(params.Beat), map[string]string{
		VersionLabelName: spec.Version})

	annotations := map[string]string{
		ConfigHashAnnotationName: fmt.Sprint(configHash.Sum32()),
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, spec.Type).
		WithLabels(labels).
		WithAnnotations(annotations).
		WithResources(defaultResources).
		WithDockerImage(spec.Image, container.ImageRepository(defaultImage, spec.Version)).
		WithArgs("-e", "-c", ConfigMountPath).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initContainers...).
		WithInitContainerDefaults()

	return builder.PodTemplate, nil
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
