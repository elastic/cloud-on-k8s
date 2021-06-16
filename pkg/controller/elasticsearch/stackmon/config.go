// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	_ "embed" // for the beats config files

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	MetricbeatConfigKey       = "metricbeat.yml"
	MetricbeatConfigMapSuffix = "metricbeat-config"

	FilebeatConfigMapSuffix = "filebeat-config"
	FilebeatConfigKey       = "filebeat.yml"
)

var (
	// Environments variables used in the beats configuration to describe how to connect to Elasticsearch.
	// Warning: they are hard-coded in the two configs below.
	EsSourceURLEnvVarKey      = "ES_SOURCE_URL"
	EsSourceURLEnvVarValue    = "https://localhost:9200"
	EsSourceUsernameEnvVarKey = "ES_SOURCE_USERNAME"
	EsSourcePasswordEnvVarKey = "ES_SOURCE_PASSWORD" //nolint:gosec
	EsTargetURLEnvVarKey      = "ES_TARGET_URL"
	EsTargetUsernameEnvVarKey = "ES_TARGET_USERNAME"
	EsTargetPasswordEnvVarKey = "ES_TARGET_PASSWORD" //nolint:gosec

	// Paths to the Elasticsearch CA certificates used by the beats to send data
	MonitoringMetricsSourceEsCaCertMountPath = "/mnt/es/monitoring/metrics/source"
	MonitoringMetricsTargetEsCaCertMountPath = "/mnt/es/monitoring/metrics/target"
	MonitoringLogsTargetEsCaCertMountPath    = "/mnt/es/monitoring/logs/target"

	// MetricbeatConfig is a static configuration for Metricbeat to collect monitoring data about Elasticsearch
	// Warning: environment variables and CA cert paths defined below are hard-coded for simplicity.
	//go:embed metricbeat.yml
	MetricbeatConfig string

	// FilebeatConfig is a static configuration for Filebeat to collect Elasticsearch logs
	// Warning: environment variables and CA cert paths defined below are hard-coded for simplicity.
	//go:embed filebeat.yml
	FilebeatConfig string
)

func metricbeatConfigMapName(es esv1.Elasticsearch) string {
	return esv1.ESNamer.Suffix(es.Name, MetricbeatConfigMapSuffix)
}

func filebeatConfigMapName(es esv1.Elasticsearch) string {
	return esv1.ESNamer.Suffix(es.Name, FilebeatConfigMapSuffix)
}

// MetricbeatConfigMapData returns the data for the ConfigMap holding the Metricbeat configuration
func MetricbeatConfigMapData(es esv1.Elasticsearch) (types.NamespacedName, map[string]string) {
	nsn := types.NamespacedName{Namespace: es.Namespace, Name: metricbeatConfigMapName(es)}
	data := map[string]string{MetricbeatConfigKey: MetricbeatConfig}
	return nsn, data
}

// FilebeatConfigMapData returns the data for the ConfigMap holding the Filebeat configuration
func FilebeatConfigMapData(es esv1.Elasticsearch) (types.NamespacedName, map[string]string) {
	nsn := types.NamespacedName{Namespace: es.Namespace, Name: filebeatConfigMapName(es)}
	data := map[string]string{FilebeatConfigKey: FilebeatConfig}
	return nsn, data
}
