// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/network"
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

func Metricbeat(client k8s.Client, kb kbv1.Kibana) (stackmon.BeatSidecar, error) {
	metricbeat, err := stackmon.NewMetricBeatSidecar(
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
		return stackmon.BeatSidecar{}, err
	}
	return metricbeat, nil
}

func Filebeat(client k8s.Client, kb kbv1.Kibana) (stackmon.BeatSidecar, error) {
	filebeat, err := stackmon.NewFileBeatSidecar(client, &kb, kb.Spec.Version, filebeatConfig, nil)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	return filebeat, nil
}

// WithMonitoring updates the Elasticsearch Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Elasticsearch pod and injects the volumes for the beat configurations and the ES CA certificates.
func WithMonitoring(client k8s.Client, builder *defaults.PodTemplateBuilder, kb kbv1.Kibana) (*defaults.PodTemplateBuilder, error) {
	// no monitoring defined, skip
	if !monitoring.IsMonitoringDefined(&kb) {
		return builder, nil
	}

	configHash := sha256.New224()
	volumes := make([]corev1.Volume, 0)

	if monitoring.IsMonitoringMetricsDefined(&kb) {
		b, err := Metricbeat(client, kb)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(b.Container)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	if monitoring.IsMonitoringLogsDefined(&kb) {
		b, err := Filebeat(client, kb)
		if err != nil {
			return nil, err
		}

		// create a logs volume shared between Kibana and Filebeat
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
