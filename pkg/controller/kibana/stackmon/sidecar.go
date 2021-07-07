// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"crypto/sha256"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	corev1 "k8s.io/api/core/v1"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// cfgHashLabel is used to store a hash of the Metricbeat and Filebeat configurations.
	// Using only one label for both configs to save labels.
	cfgHashLabel = "kibana.k8s.elastic.co/monitoring-config-hash"

	kibanaLogsVolumeName = "kibana-logs"
	kibanaLogsMountPath  = "/usr/share/kibana/logs"
	kibanaLogFilename    = "kibana.json"
)

func IsMonitoringMetricsDefined(kb kbv1.Kibana) bool {
	for _, ref := range kb.Spec.Monitoring.Metrics.ElasticsearchRefs {
		if !ref.IsDefined() {
			return false
		}
	}
	return len(kb.Spec.Monitoring.Metrics.ElasticsearchRefs) > 0
}

func IsMonitoringLogsDefined(kb kbv1.Kibana) bool {
	for _, ref := range kb.Spec.Monitoring.Logs.ElasticsearchRefs {
		if !ref.IsDefined() {
			return false
		}
	}
	return len(kb.Spec.Monitoring.Logs.ElasticsearchRefs) > 0
}

func isMonitoringDefined(kb kbv1.Kibana) bool {
	return IsMonitoringMetricsDefined(kb) || IsMonitoringLogsDefined(kb)
}

func Metricbeat(client k8s.Client, kb kbv1.Kibana) (common.BeatSidecar, error) {
	metricbeat, err := common.NewMetricBeatSidecar(
		client,
		commonv1.KbMonitoringAssociationType,
		&kb,
		kb.Spec.Version,
		kb.Spec.ElasticsearchRef.NamespacedName(),
		metricbeatConfigTemplate,
		kbv1.KBNamer,
		fmt.Sprintf("%s://localhost:%d", kb.Spec.HTTP.Protocol(), network.HTTPPort),
		kb.Spec.HTTP.TLS.Enabled(),
	)
	if err != nil {
		return common.BeatSidecar{}, err
	}
	return metricbeat, nil
}

func Filebeat(client k8s.Client, kb kbv1.Kibana) (common.BeatSidecar, error) {
	filebeat, err := common.NewFileBeatSidecar(client, &kb, kb.Spec.Version, filebeatConfig, nil)
	if err != nil {
		return common.BeatSidecar{}, err
	}

	return filebeat, nil
}

// WithMonitoring updates the Elasticsearch Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Elasticsearch pod and injects the volumes for the beat configurations and the ES CA certificates.
func WithMonitoring(client k8s.Client, builder *defaults.PodTemplateBuilder, kb kbv1.Kibana) (*defaults.PodTemplateBuilder, error) {
	// no monitoring defined, skip
	if !isMonitoringDefined(kb) {
		return builder, nil
	}

	configHash := sha256.New224()
	volumes := make([]corev1.Volume, 0)

	if IsMonitoringMetricsDefined(kb) {
		b, err := Metricbeat(client, kb)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(b.Container)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	if IsMonitoringLogsDefined(kb) {
		b, err := Filebeat(client, kb)
		if err != nil {
			return nil, err
		}

		// Create a logs volume shared between Kibana and Filebeat
		logsVolume := volume.NewEmptyDirVolume(kibanaLogsVolumeName, kibanaLogsMountPath)
		volumes = append(volumes, logsVolume.Volume())
		filebeat := b.Container
		filebeat.VolumeMounts = append(filebeat.VolumeMounts, logsVolume.VolumeMount())
		builder.WithVolumeMounts(logsVolume.VolumeMount())

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(filebeat)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	// add the config hash label to ensure pod rotation when an ES password or a CA are rotated
	builder.WithLabels(map[string]string{cfgHashLabel: fmt.Sprintf("%x", configHash.Sum(nil))})
	// inject all volumes
	builder.WithVolumes(volumes...)

	return builder, nil
}
