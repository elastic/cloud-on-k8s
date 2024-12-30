// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"fmt"
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/blang/semver/v4"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
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
	resource monitoring.HasMonitoring,
	imageVersion semver.Version,
	caVolume volume.VolumeLike,
	baseConfig string,
) (BeatSidecar, error) {
	image := container.ImageRepository(container.MetricbeatImage, imageVersion)
	// EmptyDir volume so that MetricBeat does not write in the container image, which allows ReadOnlyRootFilesystem: true
	emptyDir := volume.NewEmptyDirVolume("metricbeat-data", "/usr/share/metricbeat/data")
	return NewBeatSidecar(ctx, client, "metricbeat", image, resource, monitoring.GetMetricsAssociation(resource), baseConfig, caVolume, emptyDir)
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

// CAVolume returns a volume containing the CA certificate for the monitored resource if TLS is enabled.
// If TLS is not enabled or no (self-signed) CA is in use, it returns nil.
func CAVolume(
	c k8s.Client,
	nsn types.NamespacedName,
	namer name.Namer,
	associationType commonv1.AssociationType,
	useTLS bool,
) (volume.VolumeLike, error) {
	if !useTLS {
		return nil, nil
	}
	hasCA, err := certificates.PublicCertsHasCACert(c, namer, nsn.Namespace, nsn.Name)
	if !hasCA || err != nil {
		return nil, err
	}
	return volume.NewSecretVolumeWithMountPath(
		certificates.PublicCertsSecretName(namer, nsn.Name),
		fmt.Sprintf("%s-local-ca", string(associationType)),
		fmt.Sprintf("/mnt/elastic-internal/%s/%s/%s/certs", string(associationType), nsn.Namespace, nsn.Name),
	), nil
}

// TemplateParams are commonly used parameters to render a Beats configuration template.
// Stack monitoring implementations can choose to implement their own template parameters if needed.
type TemplateParams struct {
	URL      string
	Username string
	Password string
	IsSSL    bool
	CAVolume volume.VolumeLike
}
