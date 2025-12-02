// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// autoOpsESConfigMapName is the static name for the autoops-es-config ConfigMap
	autoOpsESConfigMapName = "autoops-es-config"
	// autoOpsESConfigFileName is the key name for the config file in the ConfigMap
	autoOpsESConfigFileName = "autoops_es.yml"
)

// autoOpsESConfigData contains the static configuration data for the autoops agent
const autoOpsESConfigData = `receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        # Metrics
        - module: autoops_es
          hosts: ${env:AUTOOPS_ES_URL}
          ssl.verification_mode: none
          period: 10s
          metricsets:
            - cat_shards
            - cluster_health
            - cluster_settings
            - license
            - node_stats
            - tasks_management
        # Templates
        - module: autoops_es
          hosts: ${env:AUTOOPS_ES_URL}
          ssl.verification_mode: none
          period: 24h
          metricsets:
            - cat_template
            - component_template
            - index_template
    processors:
      - add_fields:
          target: autoops_es
          fields:
            temp_resource_id: ${env:AUTOOPS_TEMP_RESOURCE_ID}
            token: ${env:AUTOOPS_TOKEN}
    output:
      otelconsumer: {}
    telemetry_types: ["logs"]
exporters:
  otlphttp:
    headers:
      Authorization: "AutoOpsToken ${env:AUTOOPS_TOKEN}"
    endpoint: ${env:AUTOOPS_OTEL_URL}
service:
  pipelines:
    logs:
      receivers: [metricbeatreceiver]
      exporters: [otlphttp]
  telemetry:
    logs:
      encoding: json
`

// ReconcileAutoOpsESConfigMap reconciles the ConfigMap containing the autoops configuration.
// This ConfigMap is shared by all deployments created by the controller within the same namespace.
func ReconcileAutoOpsESConfigMap(ctx context.Context, c k8s.Client, policy autoopsv1alpha1.AutoOpsAgentPolicy) error {
	expected := buildAutoOpsESConfigMap(policy)

	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &policy,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
					!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
					!reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
				reconciled.Data = expected.Data
			},
		},
	)
}

// buildAutoOpsESConfigMap builds the expected ConfigMap for autoops configuration.
func buildAutoOpsESConfigMap(policy autoopsv1alpha1.AutoOpsAgentPolicy) corev1.ConfigMap {
	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      policy.GetLabels(),
		Annotations: policy.GetAnnotations(),
	})

	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        autoOpsESConfigMapName,
			Namespace:   policy.GetNamespace(),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string]string{
			autoOpsESConfigFileName: autoOpsESConfigData,
		},
	}
}
