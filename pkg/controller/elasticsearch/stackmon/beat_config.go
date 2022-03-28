// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	_ "embed" // for the beats config files

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data about Elasticsearch
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string

	// filebeatConfig is a static configuration for Filebeat to collect Elasticsearch logs
	//go:embed filebeat.yml
	filebeatConfig string
)

// ReconcileConfigSecrets reconciles the secrets holding beats configuration
func ReconcileConfigSecrets(client k8s.Client, es esv1.Elasticsearch) error {
	isMonitoringReconcilable, err := monitoring.IsReconcilable(&es)
	if err != nil {
		return err
	}
	if !isMonitoringReconcilable {
		return nil
	}

	if monitoring.IsMetricsDefined(&es) {
		b, err := Metricbeat(client, es)
		if err != nil {
			return err
		}

		if _, err := reconciler.ReconcileSecret(client, b.ConfigSecret, &es); err != nil {
			return err
		}
	}

	if monitoring.IsLogsDefined(&es) {
		b, err := Filebeat(client, es)
		if err != nil {
			return err
		}

		if _, err := reconciler.ReconcileSecret(client, b.ConfigSecret, &es); err != nil {
			return err
		}
	}

	return nil
}
