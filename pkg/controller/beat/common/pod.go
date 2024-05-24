// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"fmt"
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	beat_stackmon "github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
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
			corev1.ResourceMemory: resource.MustParse("300Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("300Mi"),
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
		params.Context,
		params,
		&params.Beat,
		namer,
		params.Beat.GetIdentityLabels(),
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
	var sideCars []corev1.Container

	for _, v := range vols {
		volumes = append(volumes, v.Volume())
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	if keystoreResources != nil {
		_, _ = configHash.Write([]byte(keystoreResources.Hash))
		volumes = append(volumes, keystoreResources.Volume)
		initContainers = append(initContainers, keystoreResources.InitContainer)
	}

	if monitoring.IsLogsDefined(&params.Beat) {
		sideCar, err := beat_stackmon.Filebeat(params.Context, params.Client, &params.Beat, params.Beat.Spec.Version)
		if err != nil {
			return podTemplate, err
		}
		// name of container must be adjusted from default, or it will not be added to
		// pod template builder because of duplicative names.
		sideCar.Container.Name = "logs-monitoring-sidecar"
		if _, err := reconciler.ReconcileSecret(params.Context, params.Client, sideCar.ConfigSecret, &params.Beat); err != nil {
			return podTemplate, err
		}
		// Add shared volume for logs consumption.
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "filebeat-logs",
			ReadOnly:  false,
			MountPath: "/usr/share/filebeat/logs",
		})
		volumes = append(volumes, sideCar.Volumes...)
		if runningAsRoot(params.Beat) {
			sideCar.Container.SecurityContext = &corev1.SecurityContext{
				RunAsUser: ptr.To[int64](0),
			}
		}
		sideCars = append(sideCars, sideCar.Container)
	}

	if monitoring.IsMetricsDefined(&params.Beat) {
		sideCar, err := beat_stackmon.MetricBeat(params.Context, params.Client, &params.Beat, params.Beat.Spec.Version)
		if err != nil {
			return podTemplate, err
		}
		// name of container must be adjusted from default, or it will not be added to
		// pod template builder because of duplicative names.
		sideCar.Container.Name = "metrics-monitoring-sidecar"
		if _, err := reconciler.ReconcileSecret(params.Context, params.Client, sideCar.ConfigSecret, &params.Beat); err != nil {
			return podTemplate, err
		}
		// Add shared volume for Unix socket between containers.
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "shared-data",
			ReadOnly:  false,
			MountPath: "/var/shared",
		})
		volumes = append(volumes, sideCar.Volumes...)
		if runningAsRoot(params.Beat) {
			sideCar.Container.SecurityContext = &corev1.SecurityContext{
				RunAsUser: ptr.To[int64](0),
			}
		}
		sideCars = append(sideCars, sideCar.Container)
	}

	labels := maps.Merge(params.Beat.GetIdentityLabels(), map[string]string{
		VersionLabelName: spec.Version})

	annotations := map[string]string{
		ConfigHashAnnotationName: fmt.Sprint(configHash.Sum32()),
	}

	v, err := version.Parse(spec.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err // error unlikely and should have been caught during validation
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, spec.Type).
		WithLabels(labels).
		WithAnnotations(annotations).
		WithResources(defaultResources).
		WithDockerImage(spec.Image, container.ImageRepository(defaultImage, v)).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initContainers...).
		WithInitContainerDefaults().
		WithContainers(sideCars...)

	// If logs monitoring is enabled, remove the "-e" argument from the main container
	// if it exists, and do not include the "-e" startup option for the Beat so that
	// it does not log only to stderr, and writes log file for filebeat to consume.
	if monitoring.IsLogsDefined(&params.Beat) {
		if main := builder.MainContainer(); main != nil {
			removeLogToStderrOption(main)
		}
		builder = builder.WithArgs("-c", ConfigMountPath)
		return builder.PodTemplate, nil
	}

	return builder.WithArgs("-e", "-c", ConfigMountPath).PodTemplate, nil
}

func removeLogToStderrOption(container *corev1.Container) {
	for i, arg := range container.Args {
		if arg == "-e" {
			container.Args = append(container.Args[:i], container.Args[i+1:]...)
		}
	}
}

func runningAsRoot(beat beatv1beta1.Beat) bool {
	if beat.Spec.DaemonSet != nil {
		for _, container := range beat.Spec.DaemonSet.PodTemplate.Spec.Containers {
			if container.SecurityContext != nil && container.SecurityContext.RunAsUser != nil {
				if *container.SecurityContext.RunAsUser == 0 {
					return true
				}
			}
		}
	}
	if beat.Spec.Deployment != nil {
		for _, container := range beat.Spec.Deployment.PodTemplate.Spec.Containers {
			if container.SecurityContext != nil && container.SecurityContext.RunAsUser != nil {
				if *container.SecurityContext.RunAsUser == 0 {
					return true
				}
			}
		}
	}
	return false
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
