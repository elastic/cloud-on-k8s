// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// autoOpsESConfigFileName is the key name for the config file in the ConfigMap
	autoOpsESConfigFileName = "autoops_es.yml"
)

// autoOpsESConfigTemplate contains the configuration template for the autoops agent
const autoOpsESConfigTemplate = `receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        # Metrics
        - module: autoops_es
          hosts: ${env:AUTOOPS_ES_URL}
{{- if .SSLEnabled}}
          ssl.verification_mode: certificate
          ssl.certificate_authorities: ["{{ .CACertPath }}"]
{{- else}}
          ssl.verification_mode: none
{{- end}}
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
{{- if .SSLEnabled}}
          ssl.verification_mode: certificate
          ssl.certificate_authorities: ["{{ .CACertPath }}"]
{{- else}}
          ssl.verification_mode: none
{{- end}}
          period: 24h
          metricsets:
            - cat_template
            - component_template
            - index_template
    processors:
      - add_fields:
          target: autoops_es
          fields:
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
  extensions: [healthcheckv2]
  pipelines:
    logs:
      receivers: [metricbeatreceiver]
      exporters: [otlphttp]
  telemetry:
    logs:
      encoding: json
extensions:
  healthcheckv2:
    use_v2: true
    component_health:
      include_permanent_errors: true
      include_recoverable_errors: true
      recovery_duration: 5m
    http:
      endpoint: "0.0.0.0:13133"
      status:
        enabled: true
        path: "/health/status"
      config:
        enabled: true
        path: "/health/config"
`

// configTemplateData holds the data for rendering the config template
type configTemplateData struct {
	SSLEnabled bool
	CACertPath string
}

// ReconcileAutoOpsESConfigMap reconciles the ConfigMap containing the autoops configuration
// specific to each ES instance.
func ReconcileAutoOpsESConfigMap(ctx context.Context, c k8s.Client, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) error {
	expected, err := buildAutoOpsESConfigMap(policy, es)
	if err != nil {
		return err
	}

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
// SSL is enabled based on the Elasticsearch CRD's spec.http.tls configuration.
func buildAutoOpsESConfigMap(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (corev1.ConfigMap, error) {
	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      policy.GetLabels(),
		Annotations: policy.GetAnnotations(),
	})

	// Build CA certificate path if SSL is enabled
	caCertPath := ""
	sslEnabled := es.Spec.HTTP.TLS.Enabled()
	if sslEnabled {
		caCertPath = filepath.Join(
			fmt.Sprintf("/mnt/elastic-internal/es-ca/%s-%s", es.Namespace, es.Name),
			certificates.CAFileName,
		)
	}

	tmpl, err := template.New("autoops-config").Parse(autoOpsESConfigTemplate)
	if err != nil {
		return corev1.ConfigMap{}, err
	}

	var configBuf bytes.Buffer
	templateData := configTemplateData{
		SSLEnabled: sslEnabled,
		CACertPath: caCertPath,
	}
	if err := tmpl.Execute(&configBuf, templateData); err != nil {
		return corev1.ConfigMap{}, err
	}

	// Use ES-specific ConfigMap name to allow per-ES configuration
	configMapName := autoopsv1alpha1.Config(policy.GetName(), es)

	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        configMapName,
			Namespace:   policy.GetNamespace(),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string]string{
			autoOpsESConfigFileName: configBuf.String(),
		},
	}, nil
}
