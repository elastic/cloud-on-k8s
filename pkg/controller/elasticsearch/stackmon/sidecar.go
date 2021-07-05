// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// cfgHashLabel is used to store a hash of the Metricbeat and Filebeat configurations.
	// Using only one label for both configs to save labels.
	cfgHashLabel = "elasticsearch.k8s.elastic.co/monitoring-config-hash"
)

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

func isMonitoringDefined(es esv1.Elasticsearch) bool {
	return IsMonitoringMetricsDefined(es) || IsMonitoringLogsDefined(es)
}

func Metricbeat(client k8s.Client, es esv1.Elasticsearch) (common.BeatSidecar, error) {
	metricbeatConfig, sourceEsCaVolume, err := buildMetricbeatBaseConfig(client, es)
	if err != nil {
		return common.BeatSidecar{}, err
	}

	image := container.ImageRepository(container.MetricbeatImage, es.Spec.Version)
	metricbeat, err := common.NewMetricBeatSidecar(client, &es, image, metricbeatConfig, sourceEsCaVolume)
	if err != nil {
		return common.BeatSidecar{}, err
	}

	return metricbeat, nil
}

func Filebeat(client k8s.Client, es esv1.Elasticsearch) (common.BeatSidecar, error) {
	image := container.ImageRepository(container.MetricbeatImage, es.Spec.Version)
	filebeat, err := common.NewFileBeatSidecar(client, &es, image, filebeatConfig, nil)
	if err != nil {
		return common.BeatSidecar{}, err
	}

	return filebeat, nil
}

// WithMonitoring updates the Elasticsearch Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Elasticsearch pod and injects the volumes for the beat configurations and the ES CA certificates.
func WithMonitoring(client k8s.Client, builder *defaults.PodTemplateBuilder, es esv1.Elasticsearch) (*defaults.PodTemplateBuilder, error) {
	// no monitoring defined, skip
	if !isMonitoringDefined(es) {
		return builder, nil
	}

	configHash := sha256.New224()
	volumes := make([]corev1.Volume, 0)

	if IsMonitoringMetricsDefined(es) {
		b, err := Metricbeat(client, es)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(b.Container)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	if IsMonitoringLogsDefined(es) {
		// enable Stack logging to write Elasticsearch logs to disk
		builder.WithEnv(fileLogStyleEnvVar())

		b, err := Filebeat(client, es)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		filebeat := b.Container

		// share the ES logs volume into the Filebeat container
		filebeat.VolumeMounts = append(filebeat.VolumeMounts, esvolume.DefaultLogsVolumeMount)

		builder.WithContainers(filebeat)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	// add the config hash label to ensure pod rotation when an ES password or a CA are rotated
	builder.WithLabels(map[string]string{cfgHashLabel: fmt.Sprintf("%x", configHash.Sum(nil))})
	// inject all volumes
	builder.WithVolumes(volumes...)

	return builder, nil
}
