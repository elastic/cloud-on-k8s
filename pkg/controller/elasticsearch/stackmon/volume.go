// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	MetricbeatContainerName      = "metricbeat"
	MetricbeatConfigVolumeName   = "metricbeat-config"
	MetricbeatConfigDirMountPath = "/etc/metricbeat-config"

	FilebeatContainerName      = "filebeat"
	FilebeatConfigVolumeName   = "filebeat-config"
	FilebeatConfigDirMountPath = "/etc/filebeat-config"

	MonitoringMetricsSourceEsCaCertVolumeName       = "es-monitoring-metrics-source-certs"
	MonitoringMetricsTargetEsCaCertVolumeNameFormat = "es-monitoring-metrics-target-certs-%d"
	MonitoringLogsTargetEsCaCertVolumeNameFormat    = "es-monitoring-logs-target-certs-%d"
)

func metricbeatConfigMapVolume(es esv1.Elasticsearch) volume.ConfigMapVolume {
	return volume.NewConfigMapVolume(
		metricbeatConfigMapName(es),
		MetricbeatConfigVolumeName,
		MetricbeatConfigDirMountPath,
	)
}

func filebeatConfigMapVolume(es esv1.Elasticsearch) volume.ConfigMapVolume {
	return volume.NewConfigMapVolume(
		filebeatConfigMapName(es),
		FilebeatConfigVolumeName,
		FilebeatConfigDirMountPath,
	)
}

func monitoringMetricsSourceCaCertSecretVolume(es esv1.Elasticsearch) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		certificates.PublicCertsSecretName(
			esv1.ESNamer,
			es.Name,
		),
		MonitoringMetricsSourceEsCaCertVolumeName,
		MonitoringMetricsSourceEsCaCertMountPath,
	)
}

func monitoringMetricsTargetCaCertSecretVolumes(es esv1.Elasticsearch) []volume.VolumeLike {
	volumes := make([]volume.VolumeLike, 0)
	for i, assoc := range es.GetMonitoringMetricsAssociation() {
		volumes = append(volumes, volume.NewSecretVolumeWithMountPath(
			assoc.AssociationConf().GetCASecretName(),
			fmt.Sprintf(MonitoringMetricsTargetEsCaCertVolumeNameFormat, i),
			fmt.Sprintf(MonitoringMetricsTargetEsCaCertMountPath, i),
		))
	}
	return volumes
}

func monitoringLogsTargetCaCertSecretVolumes(es esv1.Elasticsearch) []volume.VolumeLike {
	volumes := make([]volume.VolumeLike, 0)
	for i, assoc := range es.GetMonitoringLogsAssociation() {
		volumes = append(volumes, volume.NewSecretVolumeWithMountPath(
			assoc.AssociationConf().GetCASecretName(),
			fmt.Sprintf(MonitoringLogsTargetEsCaCertVolumeNameFormat, i),
			fmt.Sprintf(MonitoringLogsTargetEsCaCertMountPath, i),
		))
	}
	return volumes
}
