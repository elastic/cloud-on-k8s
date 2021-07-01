// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"hash"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func NewMetricbeatBuilder(client k8s.Client, resource HasMonitoring, baseConfig string, additionalVolume volume.VolumeLike) (BeatBuilder, error) {
	return NewBeatBuilder(client, "metricbeat", container.MetricbeatImage, resource,
		resource.GetMonitoringMetricsAssociation(), baseConfig, additionalVolume)
}

func NewFilebeatBuilder(client k8s.Client, resource HasMonitoring, baseConfig string, additionalVolume volume.VolumeLike) (BeatBuilder, error) {
	return NewBeatBuilder(client, "filebeat", container.FilebeatImage, resource,
		resource.GetMonitoringLogsAssociation(), baseConfig, additionalVolume)
}

// BeatBuilder helps with building a beat sidecar container to monitor an Elastic Stack application. It focuses on
// building the container, the secret holding the configuration file and the associated volumes for the pod.
type BeatBuilder struct {
	container    corev1.Container
	configHash   hash.Hash
	configSecret corev1.Secret
	volumes      []volume.VolumeLike
}

func NewBeatBuilder(client k8s.Client, beatName string, image container.Image, resource HasMonitoring,
	associations []commonv1.Association, baseConfig string, additionalVolume volume.VolumeLike,
) (BeatBuilder, error) {
	// build the beat config
	config, err := newBeatConfig(client, beatName, resource, associations, baseConfig)
	if err != nil {
		return BeatBuilder{}, err
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

	return BeatBuilder{
		container: corev1.Container{
			Name:         beatName,
			Image:        container.ImageRepository(image, resource.Version()),
			Args:         []string{"-c", config.filepath, "-e"},
			Env:          defaults.PodDownwardEnvVars(),
			VolumeMounts: volumeMounts,
		},
		configHash:   config.hash,
		configSecret: config.secret,
		volumes:      volumes,
	}, nil
}

func (b BeatBuilder) Container() corev1.Container {
	return b.container
}

func (b BeatBuilder) ConfigHash() []byte {
	return b.configHash.Sum(nil)
}

func (b BeatBuilder) ConfigSecret() corev1.Secret {
	return b.configSecret
}

func (b BeatBuilder) Volumes() []corev1.Volume {
	volumes := make([]corev1.Volume, 0)
	for _, v := range b.volumes {
		volumes = append(volumes, v.Volume())
	}
	return volumes
}
