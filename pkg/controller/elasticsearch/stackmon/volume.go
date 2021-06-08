// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	MetricbeatContainerName      = "metricbeat"
	MetricbeatConfigVolumeName   = "metricbeat-config"
	MetricbeatConfigDirMountPath = "/etc/metricbeat-config"

	FilebeatContainerName      = "filebeat"
	FilebeatConfigVolumeName   = "filebeat-config"
	FilebeatConfigDirMountPath = "/etc/filebeat-config"

	MonitoringMetricsSourceEsCaCertVolumeName = "es-monitoring-metrics-source-certs"
	MonitoringMetricsTargetEsCaCertVolumeName = "es-monitoring-metrics-target-certs"
	MonitoringLogsTargetEsCaCertVolumeName    = "es-monitoring-logs-target-certs"
)

// monitoringVolumes returns the volumes to add to the Elasticsearch pod for the Metricbeat and Filebeat sidecar containers.
// Metricbeat mounts its configuration and the CA certificates of the source and the target Elasticsearch cluster.
// Filebeat mounts its configuration and the CA certificate of the target Elasticsearch cluster.
func monitoringVolumes(es esv1.Elasticsearch) []corev1.Volume {
	var volumes []corev1.Volume
	if IsMonitoringMetricsDefined(es) {
		volumes = append(volumes,
			metricbeatConfigMapVolume(es).Volume(),
			monitoringMetricsSourceCaCertSecretVolume(es).Volume(),
			monitoringMetricsTargetCaCertSecretVolume(es).Volume(),
		)
	}
	if IsMonitoringLogsDefined(es) {
		volumes = append(volumes,
			filebeatConfigMapVolume(es).Volume(),
			monitoringLogsTargetCaCertSecretVolume(es).Volume(),
		)
	}
	return volumes
}

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

func monitoringMetricsTargetCaCertSecretVolume(es esv1.Elasticsearch) volume.SecretVolume {
	assocConf := es.GetMonitoringMetricsAssociation().AssociationConf()
	return volume.NewSecretVolumeWithMountPath(
		assocConf.CASecretName,
		MonitoringMetricsTargetEsCaCertVolumeName,
		MonitoringMetricsTargetEsCaCertMountPath,
	)
}

func monitoringLogsTargetCaCertSecretVolume(es esv1.Elasticsearch) volume.SecretVolume {
	assocConf := es.GetMonitoringLogsAssociation().AssociationConf()
	return volume.NewSecretVolumeWithMountPath(
		assocConf.CASecretName,
		MonitoringLogsTargetEsCaCertVolumeName,
		MonitoringLogsTargetEsCaCertMountPath,
	)
}
