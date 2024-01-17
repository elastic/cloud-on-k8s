// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	_ "embed" // for the beats config files

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data about Kibana
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string

	// filebeatConfig is a static configuration for Filebeat to collect Kibana logs
	//go:embed filebeat.yml
	filebeatConfig string
)

// ReconcileConfigSecrets reconciles the secrets holding beats configuration
func ReconcileConfigSecrets(ctx context.Context, client k8s.Client, logstash logstashv1alpha1.Logstash, apiServer configs.APIServer) error {
	isMonitoringReconcilable, err := monitoring.IsReconcilable(&logstash)
	if err != nil {
		return err
	}
	if !isMonitoringReconcilable {
		return nil
	}

	if monitoring.IsMetricsDefined(&logstash) {
		b, err := Metricbeat(ctx, client, logstash, apiServer)
		if err != nil {
			return err
		}

		if _, err := reconciler.ReconcileSecret(ctx, client, b.ConfigSecret, &logstash); err != nil {
			return err
		}
	}

	if monitoring.IsLogsDefined(&logstash) {
		b, err := Filebeat(ctx, client, logstash)
		if err != nil {
			return err
		}

		if _, err := reconciler.ReconcileSecret(ctx, client, b.ConfigSecret, &logstash); err != nil {
			return err
		}
	}

	return nil
}
