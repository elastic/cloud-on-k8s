// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	_ "embed" // for the beats config files
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Environments variables and paths to the Elasticsearch CA certificates used in the beats configuration to describe
// how to connect to Elasticsearch.
// Warning: environment variables and CA cert paths defined below are also used in the embedded files.
var (
	esSourceURLEnvVarKey      = "ES_SOURCE_URL"
	esSourceURLEnvVarValue    = "https://localhost:9200"
	esSourceUsernameEnvVarKey = "ES_SOURCE_USERNAME"
	esSourcePasswordEnvVarKey = "ES_SOURCE_PASSWORD" //nolint:gosec

	esTargetURLEnvVarKeyFormat      = "ES_%d_TARGET_URL"
	esTargetUsernameEnvVarKeyFormat = "ES_%d_TARGET_USERNAME"
	esTargetPasswordEnvVarKeyFormat = "ES_%d_TARGET_PASSWORD" //nolint:gosec

	monitoringMetricsSourceEsCaCertMountPath       = "/mnt/es/monitoring/metrics/source"
	monitoringMetricsTargetEsCaCertMountPathFormat = "/mnt/es/%d/monitoring/metrics/target"
	monitoringLogsTargetEsCaCertMountPathFormat    = "/mnt/es/%d/monitoring/logs/target"

	// metricbeatConfig is a static configuration for Metricbeat to collect monitoring data about Elasticsearch
	//go:embed metricbeat.yml
	metricbeatConfig string

	// filebeatConfig is a static configuration for Filebeat to collect Elasticsearch logs
	//go:embed filebeat.yml
	filebeatConfig string
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

// ReconcileConfigSecrets reconciles the secrets holding the beats configuration
func ReconcileConfigSecrets(client k8s.Client, es esv1.Elasticsearch) error {
	if IsMonitoringMetricsDefined(es) {
		secret := beatConfigSecret(es, metricbeatConfigSecretName, metricbeatConfigKey, metricbeatConfig)
		if _, err := reconciler.ReconcileSecret(client, secret, &es); err != nil {
			return err
		}
	}

	if IsMonitoringLogsDefined(es) {
		secret := beatConfigSecret(es, filebeatConfigSecretName, filebeatConfigKey, filebeatConfig)
		if _, err := reconciler.ReconcileSecret(client, secret, &es); err != nil {
			return err
		}
	}
	return nil
}

// beatConfigSecret returns the data for a Secret holding a beat configuration
func beatConfigSecret(
	es esv1.Elasticsearch,
	secretNamer func(es esv1.Elasticsearch) string,
	beatConfigKey string,
	beatConfig string,
) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretNamer(es),
			Namespace: es.GetNamespace(),
			Labels:    label.NewLabels(k8s.ExtractNamespacedName(&es)),
		},
		Data: map[string][]byte{
			beatConfigKey: []byte(beatConfig),
		},
	}
}

// monitoringSourceEnvVars returns the environment variables describing how to connect to the monitored Elasticsearch cluster
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

// monitoringTargetEnvVars returns the environment variables describing how to connect to Elasticsearch clusters
// referenced in the given associations
func monitoringTargetEnvVars(assocs []commonv1.Association) []corev1.EnvVar {
	vars := make([]corev1.EnvVar, 0)
	for i, assoc := range assocs {
		assocConf := assoc.AssociationConf()
		vars = append(vars, []corev1.EnvVar{
			{Name: fmt.Sprintf(esTargetURLEnvVarKeyFormat, i), Value: assocConf.GetURL()},
			{Name: fmt.Sprintf(esTargetUsernameEnvVarKeyFormat, i), Value: assocConf.GetAuthSecretKey()},
			{Name: fmt.Sprintf(esTargetPasswordEnvVarKeyFormat, i), ValueFrom: &corev1.EnvVarSource{
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
