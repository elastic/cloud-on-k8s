// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	_ "embed" // for the beats config files

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var (
	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data about Kibana
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string

	// filebeatConfig is a static configuration for Filebeat to collect Kibana logs
	//go:embed filebeat.yml
	filebeatConfig string

	// elasticAgentMetricsConfigTemplate is a configuration template for Elastic Agent to collect Kibana metrics
	//go:embed elastic-agent-metrics.tpl.yml
	elasticAgentMetricsConfigTemplate string

	// elasticAgentLogsConfig is a static configuration for Elastic Agent to collect Kibana logs
	//go:embed elastic-agent-logs.yml
	elasticAgentLogsConfig string
)

// ReconcileConfigSecrets reconciles the secrets holding the monitoring sidecar configuration
func ReconcileConfigSecrets(ctx context.Context, client k8s.Client, kb kbv1.Kibana, basePath string, meta metadata.Metadata) error {
	isMonitoringReconcilable, err := monitoring.IsReconcilable(&kb)
	if err != nil {
		return err
	}
	if !isMonitoringReconcilable {
		return nil
	}

	if monitoring.IsMetricsDefined(&kb) {
		var b stackmon.BeatSidecar
		if kb.Spec.Monitoring.ElasticAgent {
			b, err = ElasticAgentMetrics(ctx, client, kb, basePath, meta)
		} else {
			b, err = Metricbeat(ctx, client, kb, basePath, meta)
		}
		if err != nil {
			return err
		}
		if _, err := reconciler.ReconcileSecret(ctx, client, b.ConfigSecret, &kb); err != nil {
			return err
		}
	}

	if monitoring.IsLogsDefined(&kb) {
		var b stackmon.BeatSidecar
		if kb.Spec.Monitoring.ElasticAgent {
			b, err = ElasticAgentLogs(ctx, client, kb, meta)
		} else {
			b, err = Filebeat(ctx, client, kb, meta)
		}
		if err != nil {
			return err
		}
		if _, err := reconciler.ReconcileSecret(ctx, client, b.ConfigSecret, &kb); err != nil {
			return err
		}
	}

	return nil
}
