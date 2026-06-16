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
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// autoOpsBaselineConfig holds the user-tunable defaults. Users may override any field here
// via spec.config or spec.configRef. Everything the operator must always control lives in
// autoOpsMandatoryConfigTemplate and is re-applied last.
const autoOpsBaselineConfig = `
exporters:
  otlphttp:
    sending_queue:
      batch:
        flush_timeout: 11s
        min_size: 1048576
        max_size: 4194304
        sizer: bytes
      block_on_overflow: true
      enabled: true
      queue_size: 52428800
      sizer: bytes
`

// autoOpsMandatoryConfigTemplate is rendered per-ES (SSL context required) and merged last
// so its values always take precedence over the baseline and any user-supplied config.
const autoOpsMandatoryConfigTemplate = `
{{- define "ssl" -}}
{{- if .SSLEnabled}}
          ssl.verification_mode: certificate
          ssl.certificate_authorities: ["{{ .CACertPath }}"]
{{- if .ClientCertPath}}
          ssl.certificate: "{{ .ClientCertPath }}"
          ssl.key: "{{ .ClientKeyPath }}"
{{- end}}
{{- else}}
          ssl.verification_mode: none
{{- end}}
{{- end -}}
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        # Metrics
        - module: autoops_es
          hosts: ${env:AUTOOPS_ES_URL}
{{- template "ssl" . }}
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
{{- template "ssl" . }}
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
	SSLEnabled     bool
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
}

// ReconcileAutoOpsESConfigMap reconciles the ConfigMap containing the autoops configuration
// specific to each ES instance. This also returns the config hash of the ConfigMap to avoid
// retrieving it from the cache later and delaying the initial deployment.
func (r *AgentPolicyReconciler) ReconcileAutoOpsESConfigMap(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (*corev1.ConfigMap, error) {
	// Parse the inline config. NewCanonicalConfigFrom returns an empty config when Data is nil.
	specConfig := policy.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}
	userCfg, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		msg := fmt.Sprintf("unable to parse spec.config: %s", err)
		k8s.EmitEvent(r.Recorder(), &policy, corev1.EventTypeWarning, events.EventReasonValidation, events.EventActionAutoOpsReconciliation, msg)
		return nil, fmt.Errorf("while parsing spec.config: %w", err)
	}

	// Always call ParseConfigRef regardless of whether ConfigRef is set.
	// It manages the dynamic watch lifecycle: registers when set, deregisters when cleared.
	userSecretCfg, err := common.ParseConfigRef(r, &policy, policy.Spec.ConfigRef, autoopsv1alpha1.ConfigFileName)
	if err != nil {
		return nil, fmt.Errorf("while parsing spec.configRef: %w", err)
	}

	expected, err := buildAutoOpsESConfigMap(policy, es, userCfg, userSecretCfg)
	if err != nil {
		return nil, err
	}

	reconciled := &corev1.ConfigMap{}
	err = reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     r.Client,
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
	if err != nil {
		return nil, err
	}

	return reconciled, nil
}

// buildAutoOpsESConfigMap builds the expected ConfigMap for autoops configuration.
// Merge order: baselineCfg (from template) → userCfg → userSecretCfg → mandatoryCfg.
// The mandatory config is applied last so operator-owned scalars always take precedence.
func buildAutoOpsESConfigMap(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch, userCfg, userSecretCfg *settings.CanonicalConfig) (corev1.ConfigMap, error) {
	labels := resourceLabelsFor(policy, es)
	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      maps.Merge(policy.GetLabels(), labels),
		Annotations: policy.GetAnnotations(),
	})
	meta.Labels[commonv1.RestrictWatchedResourcesLabelName] = commonv1.RestrictWatchedResourcesLabelValue

	// Build CA certificate path if SSL is enabled
	caCertPath := ""
	sslEnabled := es.Spec.HTTP.TLS.Enabled()
	if sslEnabled {
		caCertPath = filepath.Join(
			fmt.Sprintf("/mnt/elastic-internal/es-ca/%s-%s", es.Namespace, es.Name),
			certificates.CAFileName,
		)
	}

	var clientCertPath, clientKeyPath string
	if annotation.HasClientAuthenticationRequired(&es) {
		clientCertDir := fmt.Sprintf("/mnt/elastic-internal/es-client-cert/%s-%s", es.Namespace, es.Name)
		clientCertPath = filepath.Join(clientCertDir, certificates.CertFileName)
		clientKeyPath = filepath.Join(clientCertDir, certificates.KeyFileName)
	}

	templateData := configTemplateData{
		SSLEnabled:     sslEnabled,
		CACertPath:     caCertPath,
		ClientCertPath: clientCertPath,
		ClientKeyPath:  clientKeyPath,
	}

	tmpl, err := template.New("autoops-mandatory").Parse(autoOpsMandatoryConfigTemplate)
	if err != nil {
		return corev1.ConfigMap{}, err
	}
	var mandatoryBuf bytes.Buffer
	if err := tmpl.Execute(&mandatoryBuf, templateData); err != nil {
		return corev1.ConfigMap{}, err
	}
	mandatoryCfg, err := settings.ParseConfig(mandatoryBuf.Bytes())
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while parsing mandatory config: %w", err)
	}

	baselineCfg, err := settings.ParseConfig([]byte(autoOpsBaselineConfig))
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while parsing baseline config: %w", err)
	}

	// Merge order: baseline → user → userSecret → mandatory.
	// User config can override anything in the baseline (e.g. sending_queue tuning).
	// Mandatory config is rendered last so all operator-owned fields always win.
	if err := baselineCfg.MergeWith(userCfg, userSecretCfg, mandatoryCfg); err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while merging config: %w", err)
	}

	rendered, err := baselineCfg.Render()
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while rendering config: %w", err)
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
			autoopsv1alpha1.ConfigFileName: string(rendered),
		},
	}, nil
}
