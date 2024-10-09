// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"hash"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func NewMetricBeatSidecar(
	ctx context.Context,
	client k8s.Client,
	associationType commonv1.AssociationType,
	resource monitoring.HasMonitoring,
	imageVersion string,
	baseConfigTemplate string,
	namer name.Namer,
	url string,
	username string,
	password string,
	isTLS bool,
) (BeatSidecar, error) {
	v, err := version.Parse(imageVersion)
	if err != nil {
		return BeatSidecar{}, err // error unlikely and should have been caught during validation
	}

	baseConfig, sourceCaVolume, err := buildMetricbeatBaseConfig(
		client,
		associationType,
		k8s.ExtractNamespacedName(resource),
		namer,
		url,
		username,
		password,
		isTLS,
		baseConfigTemplate,
		v,
	)
	if err != nil {
		return BeatSidecar{}, err
	}

	image := container.ImageRepository(container.MetricbeatImage, v)

	// EmptyDir volume so that MetricBeat does not write in the container image, which allows ReadOnlyRootFilesystem: true
	emptyDir := volume.NewEmptyDirVolume("metricbeat-data", "/usr/share/metricbeat/data")
	return NewBeatSidecar(ctx, client, "metricbeat", image, resource, monitoring.GetMetricsAssociation(resource), baseConfig, sourceCaVolume, emptyDir)
}

func NewFileBeatSidecar(ctx context.Context, client k8s.Client, resource monitoring.HasMonitoring, imageVersion string, baseConfig string, additionalVolume volume.VolumeLike) (BeatSidecar, error) {
	v, err := version.Parse(imageVersion)
	if err != nil {
		return BeatSidecar{}, err // error unlikely and should have been caught during validation
	}
	image := container.ImageRepository(container.FilebeatImage, v)
	// EmptyDir volume so that FileBeat does not write in the container image, which allows ReadOnlyRootFilesystem: true
	emptyDir := volume.NewEmptyDirVolume("filebeat-data", "/usr/share/filebeat/data")
	return NewBeatSidecar(ctx, client, "filebeat", image, resource, monitoring.GetLogsAssociation(resource), baseConfig, additionalVolume, emptyDir)
}

// BeatSidecar helps with building a beat sidecar container to monitor an Elastic Stack application. It focuses on
// building the container, the secret holding the configuration file and the associated volumes for the pod.
type BeatSidecar struct {
	Container    corev1.Container
	ConfigHash   hash.Hash
	ConfigSecret corev1.Secret
	Volumes      []corev1.Volume
}

func NewBeatSidecar(ctx context.Context, client k8s.Client, beatName string, image string, resource monitoring.HasMonitoring,
	associations []commonv1.Association, baseConfig string, additionalVolumes ...volume.VolumeLike,
) (BeatSidecar, error) {
	// build the beat config
	config, err := newBeatConfig(ctx, client, beatName, resource, associations, baseConfig)
	if err != nil {
		return BeatSidecar{}, err
	}

	// add additional volume (ex: CA volume of the monitored ES for Metricbeat)
	volumes := config.volumes
	for _, additionalVolume := range additionalVolumes {
		if additionalVolume != nil {
			volumes = append(volumes, additionalVolume)
		}
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
