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
	metricbeatConfigVolumeName   = "metricbeat-config"
	metricbeatConfigDirMountPath = "/etc/metricbeat-config"

	filebeatConfigVolumeName   = "filebeat-config"
	filebeatConfigDirMountPath = "/etc/filebeat-config"

	monitoringMetricsSourceEsCaCertVolumeName       = "es-monitoring-metrics-source-certs"
	monitoringMetricsTargetEsCaCertVolumeNameFormat = "es-monitoring-metrics-target-certs-%d"
	monitoringLogsTargetEsCaCertVolumeNameFormat    = "es-monitoring-logs-target-certs-%d"
)

func metricbeatConfigSecretName(es esv1.Elasticsearch) string {
	return esv1.ESNamer.Suffix(es.Name, metricbeatConfigVolumeName)
}

func metricbeatConfigSecretVolume(es esv1.Elasticsearch) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		metricbeatConfigSecretName(es),
		metricbeatConfigVolumeName,
		metricbeatConfigDirMountPath,
	)
}

func filebeatConfigSecretName(es esv1.Elasticsearch) string {
	return esv1.ESNamer.Suffix(es.Name, filebeatConfigVolumeName)
}

func filebeatConfigSecretVolume(es esv1.Elasticsearch) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		filebeatConfigSecretName(es),
		filebeatConfigVolumeName,
		filebeatConfigDirMountPath,
	)
}

func monitoringMetricsSourceCaCertSecretVolume(es esv1.Elasticsearch) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
		monitoringMetricsSourceEsCaCertVolumeName,
		monitoringMetricsSourceEsCaCertMountPath,
	)
}

func monitoringMetricsTargetCaCertSecretVolumes(es esv1.Elasticsearch) []volume.VolumeLike {
	volumes := make([]volume.VolumeLike, 0)
	for i, assoc := range es.GetMonitoringMetricsAssociation() {
		volumes = append(volumes, volume.NewSecretVolumeWithMountPath(
			assoc.AssociationConf().GetCASecretName(),
			fmt.Sprintf(monitoringMetricsTargetEsCaCertVolumeNameFormat, i),
			fmt.Sprintf(monitoringMetricsTargetEsCaCertMountPathFormat, i),
		))
	}
	return volumes
}

func monitoringLogsTargetCaCertSecretVolumes(es esv1.Elasticsearch) []volume.VolumeLike {
	volumes := make([]volume.VolumeLike, 0)
	for i, assoc := range es.GetMonitoringLogsAssociation() {
		volumes = append(volumes, volume.NewSecretVolumeWithMountPath(
			assoc.AssociationConf().GetCASecretName(),
			fmt.Sprintf(monitoringLogsTargetEsCaCertVolumeNameFormat, i),
			fmt.Sprintf(monitoringLogsTargetEsCaCertMountPathFormat, i),
		))
	}
	return volumes
}
