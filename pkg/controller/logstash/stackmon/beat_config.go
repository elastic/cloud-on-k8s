// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	_ "embed" // for the beats config files

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var (
	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data about Logstash
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string

	// filebeatConfig is a static configuration for Filebeat to collect Logstash logs
	//go:embed filebeat.yml
	filebeatConfig string

	// elasticAgentMetricsConfigTemplate is a configuration template for Elastic Agent to collect Logstash metrics
	//go:embed elastic-agent-metrics.tpl.yml
	elasticAgentMetricsConfigTemplate string

	// elasticAgentLogsConfig is a static configuration for Elastic Agent to collect Logstash logs
	//go:embed elastic-agent-logs.yml
	elasticAgentLogsConfig string
)

// ReconcileConfigSecrets reconciles the secrets holding the monitoring sidecar configuration
func ReconcileConfigSecrets(ctx context.Context, client k8s.Client, logstash logstashv1alpha1.Logstash, apiServer configs.APIServer, meta metadata.Metadata) error {
	isMonitoringReconcilable, err := monitoring.IsReconcilable(&logstash)
	if err != nil {
		return err
	}
	if !isMonitoringReconcilable {
		return nil
	}

	if monitoring.IsMetricsDefined(&logstash) {
		var b stackmon.BeatSidecar
		if logstash.Spec.Monitoring.ElasticAgent {
			b, err = ElasticAgentMetrics(ctx, client, logstash, apiServer, meta)
		} else {
			b, err = Metricbeat(ctx, client, logstash, apiServer, meta)
		}
		if err != nil {
			return err
		}
		if _, err := reconciler.ReconcileSecret(ctx, client, b.ConfigSecret, &logstash); err != nil {
			return err
		}
	}

	if monitoring.IsLogsDefined(&logstash) {
		var b stackmon.BeatSidecar
		if logstash.Spec.Monitoring.ElasticAgent {
			b, err = ElasticAgentLogs(ctx, client, logstash, meta)
		} else {
			b, err = Filebeat(ctx, client, logstash, meta)
		}
		if err != nil {
			return err
		}
		if _, err := reconciler.ReconcileSecret(ctx, client, b.ConfigSecret, &logstash); err != nil {
			return err
		}
	}

	return nil
}
