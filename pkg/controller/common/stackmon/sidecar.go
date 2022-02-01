// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func NewMetricBeatSidecar(
	client k8s.Client,
	associationType commonv1.AssociationType,
	resource monitoring.HasMonitoring,
	version string,
	esNsn types.NamespacedName,
	baseConfigTemplate string,
	namer name.Namer,
	url string,
	isTLS bool,
	hasCA bool,
) (BeatSidecar, error) {
	baseConfig, sourceCaVolume, err := buildMetricbeatBaseConfig(
		client,
		associationType,
		k8s.ExtractNamespacedName(resource),
		esNsn,
		namer,
		url,
		isTLS,
		hasCA,
		baseConfigTemplate,
	)
	if err != nil {
		return BeatSidecar{}, err
	}
	image := container.ImageRepository(container.MetricbeatImage, version)
	return NewBeatSidecar(client, "metricbeat", image, resource, monitoring.GetMetricsAssociation(resource), baseConfig, sourceCaVolume)
}

func NewFileBeatSidecar(client k8s.Client, resource monitoring.HasMonitoring, version string, baseConfig string, additionalVolume volume.VolumeLike) (BeatSidecar, error) {
	image := container.ImageRepository(container.FilebeatImage, version)
	return NewBeatSidecar(client, "filebeat", image, resource, monitoring.GetLogsAssociation(resource), baseConfig, additionalVolume)
}

// BeatSidecar helps with building a beat sidecar container to monitor an Elastic Stack application. It focuses on
// building the container, the secret holding the configuration file and the associated volumes for the pod.
type BeatSidecar struct {
	Container    corev1.Container
	ConfigHash   hash.Hash
	ConfigSecret corev1.Secret
	Volumes      []corev1.Volume
}

func NewBeatSidecar(client k8s.Client, beatName string, image string, resource monitoring.HasMonitoring,
	associations []commonv1.Association, baseConfig string, additionalVolume volume.VolumeLike,
) (BeatSidecar, error) {
	// build the beat config
	config, err := newBeatConfig(client, beatName, resource, associations, baseConfig)
	if err != nil {
		return BeatSidecar{}, err
	}

	// add additional volume (ex: CA volume of the monitored ES for Metricbeat)
	volumes := config.volumes
	if additionalVolume != nil {
		volumes = append(volumes, additionalVolume)
	}

	// prepare the volume mounts for the beat container from all provided volumes
	volumeMounts := make([]corev1.VolumeMount, len(volumes))
	for i, v := range volumes {
		volumeMounts[i] = v.VolumeMount()
	}

	// prepare the volumes for the pod
	podVolumes := make([]corev1.Volume, 0)
	for _, v := range volumes {
		podVolumes = append(podVolumes, v.Volume())
	}

	return BeatSidecar{
		Container: corev1.Container{
			Name:         beatName,
			Image:        image,
			Args:         []string{"-c", config.filepath, "-e"},
			Env:          defaults.PodDownwardEnvVars(),
			VolumeMounts: volumeMounts,
		},
		ConfigHash:   config.hash,
		ConfigSecret: config.secret,
		Volumes:      podVolumes,
	}, nil
}
