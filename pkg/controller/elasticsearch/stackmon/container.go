// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"path/filepath"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	metricbeatContainerName = "metricbeat"
	filebeatContainerName   = "filebeat"

	metricbeatConfigKey = "metricbeat.yml"
	filebeatConfigKey   = "filebeat.yml"
)

func IsStackMonitoringDefined(es esv1.Elasticsearch) bool {
	return IsMonitoringMetricsDefined(es) || IsMonitoringLogsDefined(es)
}

func IsMonitoringMetricsDefined(es esv1.Elasticsearch) bool {
	for _, ref := range es.Spec.Monitoring.Metrics.ElasticsearchRefs {
		if !ref.IsDefined() {
			return false
		}
	}
	return len(es.Spec.Monitoring.Metrics.ElasticsearchRefs) > 0
}

func IsMonitoringLogsDefined(es esv1.Elasticsearch) bool {
	for _, ref := range es.Spec.Monitoring.Logs.ElasticsearchRefs {
		if !ref.IsDefined() {
			return false
		}
	}
	return len(es.Spec.Monitoring.Logs.ElasticsearchRefs) > 0
}

// WithMonitoring updates the Elasticsearch Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Elasticsearch pod and injects volumes for Metricbeat/Filebeat configs and ES source/target CA certs.
func WithMonitoring(builder *defaults.PodTemplateBuilder, es esv1.Elasticsearch) (*defaults.PodTemplateBuilder, error) {
	// No monitoring defined, skip
	if !IsStackMonitoringDefined(es) {
		return builder, nil
	}

	volumeLikes := make([]volume.VolumeLike, 0)

	if IsMonitoringMetricsDefined(es) {
		metricbeatVolumes := append(
			monitoringMetricsTargetCaCertSecretVolumes(es),
			metricbeatConfigSecretVolume(es),
			monitoringMetricsSourceCaCertSecretVolume(es),
		)
		volumeLikes = append(volumeLikes, metricbeatVolumes...)

		// Inject Metricbeat sidecar container
		builder.WithContainers(metricbeatContainer(es, metricbeatVolumes))
	}

	if IsMonitoringLogsDefined(es) {
		// Enable Stack logging to write Elasticsearch logs to disk
		builder.WithEnv(fileLogStyleEnvVar())

		filebeatVolumes := append(
			monitoringLogsTargetCaCertSecretVolumes(es),
			filebeatConfigSecretVolume(es),
		)
		volumeLikes = append(volumeLikes, filebeatVolumes...)

		// Inject Filebeat sidecar container
		builder.WithContainers(filebeatContainer(es, filebeatVolumes))
	}

	// Inject volumes
	volumes := make([]corev1.Volume, 0)
	for _, v := range volumeLikes {
		volumes = append(volumes, v.Volume())
	}
	builder.WithVolumes(volumes...)

	return builder, nil
}

func metricbeatContainer(es esv1.Elasticsearch, volumes []volume.VolumeLike) corev1.Container {
	volumeMounts := make([]corev1.VolumeMount, 0)
	for _, v := range volumes {
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	envVars := append(monitoringSourceEnvVars(es), monitoringTargetEnvVars(es.GetMonitoringMetricsAssociation())...)

	return corev1.Container{
		Name:         metricbeatContainerName,
		Image:        container.ImageRepository(container.MetricbeatImage, es.Spec.Version),
		Args:         []string{"-c", filepath.Join(metricbeatConfigDirMountPath, metricbeatConfigKey), "-e"},
		Env:          append(envVars, defaults.PodDownwardEnvVars()...),
		VolumeMounts: volumeMounts,
	}
}

func filebeatContainer(es esv1.Elasticsearch, volumes []volume.VolumeLike) corev1.Container {
	volumeMounts := []corev1.VolumeMount{
		esvolume.DefaultLogsVolumeMount, // mount Elasticsearch logs volume into the Filebeat container
	}
	for _, v := range volumes {
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	envVars := monitoringTargetEnvVars(es.GetMonitoringLogsAssociation())

	return corev1.Container{
		Name:         filebeatContainerName,
		Image:        container.ImageRepository(container.FilebeatImage, es.Spec.Version),
		Args:         []string{"-c", filepath.Join(filebeatConfigDirMountPath, filebeatConfigKey), "-e"},
		Env:          append(envVars, defaults.PodDownwardEnvVars()...),
		VolumeMounts: volumeMounts,
	}
}
