// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"errors"
	"path/filepath"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	esLogStyleEnvVarKey   = "ES_LOG_STYLE"
	esLogStyleEnvVarValue = "file"

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
	isMonitoringMetrics := IsMonitoringMetricsDefined(es)
	isMonitoringLogs := IsMonitoringLogsDefined(es)

	// No monitoring defined, skip
	if !isMonitoringMetrics && !isMonitoringLogs {
		return builder, nil
	}

	volumeLikes := make([]volume.VolumeLike, 0)

	if isMonitoringMetrics {
		metricbeatVolumes := append(
			monitoringMetricsTargetCaCertSecretVolumes(es),
			metricbeatConfigSecretVolume(es),
			monitoringMetricsSourceCaCertSecretVolume(es),
		)
		volumeLikes = append(volumeLikes, metricbeatVolumes...)

		// Inject Metricbeat sidecar container
		metricbeat, err := metricbeatContainer(es, metricbeatVolumes)
		if err != nil {
			return nil, err
		}
		builder.WithContainers(metricbeat)
	}

	if isMonitoringLogs {
		// Enable Stack logging to write Elasticsearch logs to disk
		builder.WithEnv(stackLoggingEnvVar())

		filebeatVolumes := append(
			monitoringLogsTargetCaCertSecretVolumes(es),
			filebeatConfigSecretVolume(es),
		)
		volumeLikes = append(volumeLikes, filebeatVolumes...)

		// Inject Filebeat sidecar container
		filebeat, err := filebeatContainer(es, filebeatVolumes)
		if err != nil {
			return nil, err
		}
		builder.WithContainers(filebeat)
	}

	// Inject volumes
	volumes := make([]corev1.Volume, 0)
	for _, v := range volumeLikes {
		volumes = append(volumes, v.Volume())
	}
	builder.WithVolumes(volumes...)

	return builder, nil
}

func stackLoggingEnvVar() corev1.EnvVar {
	return corev1.EnvVar{Name: esLogStyleEnvVarKey, Value: esLogStyleEnvVarValue}
}

func metricbeatContainer(es esv1.Elasticsearch, volumes []volume.VolumeLike) (corev1.Container, error) {
	image, err := fullContainerImage(es, container.MetricbeatImage)
	if err != nil {
		return corev1.Container{}, err
	}

	volumeMounts := make([]corev1.VolumeMount, 0)
	for _, v := range volumes {
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	envVars := append(monitoringSourceEnvVars(es), monitoringTargetEnvVars(es.GetMonitoringMetricsAssociation())...)

	return corev1.Container{
		Name:         metricbeatContainerName,
		Image:        image,
		Args:         []string{"-c", filepath.Join(metricbeatConfigDirMountPath, metricbeatConfigKey), "-e"},
		Env:          append(envVars, defaults.PodDownwardEnvVars()...),
		VolumeMounts: volumeMounts,
	}, nil
}

func filebeatContainer(es esv1.Elasticsearch, volumes []volume.VolumeLike) (corev1.Container, error) {
	image, err := fullContainerImage(es, container.FilebeatImage)
	if err != nil {
		return corev1.Container{}, err
	}

	volumeMounts := []corev1.VolumeMount{
		esvolume.DefaultLogsVolumeMount, // mount Elasticsearch logs volume into the Filebeat container
	}
	for _, v := range volumes {
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	envVars := monitoringTargetEnvVars(es.GetMonitoringLogsAssociation())

	return corev1.Container{
		Name:         filebeatContainerName,
		Image:        image,
		Args:         []string{"-c", filepath.Join(filebeatConfigDirMountPath, filebeatConfigKey), "-e"},
		Env:          append(envVars, defaults.PodDownwardEnvVars()...),
		VolumeMounts: volumeMounts,
	}, nil
}

// fullContainerImage returns the full Beat container image with the image registry.
// If the Elasticsearch specification is configured with a custom image, we do best effort by trying to derive the Beat
// image from the Elasticsearch custom image with an image name replacement
// (<registry>/elasticsearch/elasticsearch:<version> becomes <registry>/beats/<filebeat|metricbeat>:<version>)
func fullContainerImage(es esv1.Elasticsearch, defaultImage container.Image) (string, error) {
	fullCustomImage := es.Spec.Image
	if fullCustomImage != "" {
		esImage := string(container.ElasticsearchImage)
		// Check if Elasticsearch image follows official Elastic naming
		if strings.Contains(fullCustomImage, esImage) {
			// Derive the Beat image from the ES custom image, there is no guarantee that the resulted image exists
			return strings.ReplaceAll(fullCustomImage, esImage, string(defaultImage)), nil
		}
		return "", errors.New("stack monitoring not supported with custom image")
	}
	return container.ImageRepository(defaultImage, es.Spec.Version), nil
}

func monitoringSourceEnvVars(es esv1.Elasticsearch) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: esSourceURLEnvVarKey, Value: esSourceURLEnvVarValue},
		{Name: esSourceUsernameEnvVarKey, Value: user.ElasticUserName},
		{Name: esSourcePasswordEnvVarKey, ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: esv1.ElasticUserSecret(es.Name)},
				Key: user.ElasticUserName,
			},
		}},
	}
}
