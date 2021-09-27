// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
)

// MonitoringConfig returns the Elasticsearch settings to enable the collection of monitoring data
func MonitoringConfig(es esv1.Elasticsearch) commonv1.Config {
	if !monitoring.IsMetricsDefined(&es) {
		return commonv1.Config{}
	}
	return commonv1.Config{Data: map[string]interface{}{
		esv1.XPackMonitoringCollectionEnabled:              true,
		esv1.XPackMonitoringElasticsearchCollectionEnabled: false,
	}}
}

// fileLogStyleEnvVar returns the environment variable to configure the Elasticsearch container to write logs to disk
func fileLogStyleEnvVar() corev1.EnvVar {
	return corev1.EnvVar{Name: "ES_LOG_STYLE", Value: "file"}
}
