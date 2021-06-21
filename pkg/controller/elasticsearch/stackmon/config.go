// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	_ "embed" // for the beats config files
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	MetricbeatConfigKey       = "metricbeat.yml"
	MetricbeatConfigMapSuffix = "metricbeat-config"

	FilebeatConfigMapSuffix = "filebeat-config"
	FilebeatConfigKey       = "filebeat.yml"
)

// Environments variables and paths to the Elasticsearch CA certificates used in the beats configuration to describe
// how to connect to Elasticsearch.
// Warning: environment variables and CA cert paths defined below are also used in the embedded files.
var (
	EsSourceURLEnvVarKey      = "ES_SOURCE_URL"
	EsSourceURLEnvVarValue    = "https://localhost:9200"
	EsSourceUsernameEnvVarKey = "ES_SOURCE_USERNAME"
	EsSourcePasswordEnvVarKey = "ES_SOURCE_PASSWORD" //nolint:gosec

	EsTargetURLEnvVarKeyFormat      = "ES_%d_TARGET_URL"
	EsTargetUsernameEnvVarKeyFormat = "ES_%d_TARGET_USERNAME"
	EsTargetPasswordEnvVarKeyFormat = "ES_%d_TARGET_PASSWORD" //nolint:gosec

	MonitoringMetricsSourceEsCaCertMountPath = "/mnt/es/monitoring/metrics/source"
	MonitoringMetricsTargetEsCaCertMountPath = "/mnt/es/%d/monitoring/metrics/target"
	MonitoringLogsTargetEsCaCertMountPath    = "/mnt/es/%d/monitoring/logs/target"

	// MetricbeatConfig is a static configuration for Metricbeat to collect monitoring data about Elasticsearch
	//go:embed metricbeat.yml
	MetricbeatConfig string

	// FilebeatConfig is a static configuration for Filebeat to collect Elasticsearch logs
	// Warning: environment variables and CA cert paths defined below are hard-coded for simplicity.
	//go:embed filebeat.yml
	FilebeatConfig string
)

// MonitoringConfig returns the Elasticsearch settings required to enable the collection of monitoring data
func MonitoringConfig(es esv1.Elasticsearch) commonv1.Config {
	if !IsMonitoringMetricsDefined(es) {
		return commonv1.Config{}
	}
	return commonv1.Config{Data: map[string]interface{}{
		esv1.XPackMonitoringCollectionEnabled:              true,
		esv1.XPackMonitoringElasticsearchCollectionEnabled: false,
	}}
}

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

func monitoringTargetEnvVars(assocs []commonv1.Association) []corev1.EnvVar {
	vars := make([]corev1.EnvVar, 0)
	for i, assoc := range assocs {
		assocConf := assoc.AssociationConf()
		vars = append(vars, []corev1.EnvVar{
			{Name: fmt.Sprintf(EsTargetURLEnvVarKeyFormat, i), Value: assocConf.GetURL()},
			{Name: fmt.Sprintf(EsTargetUsernameEnvVarKeyFormat, i), Value: assocConf.GetAuthSecretKey()},
			{Name: fmt.Sprintf(EsTargetPasswordEnvVarKeyFormat, i), ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: assocConf.GetAuthSecretName(),
					},
					Key: assocConf.GetAuthSecretKey(),
				},
			}}}...,
		)
	}
	return vars
}
