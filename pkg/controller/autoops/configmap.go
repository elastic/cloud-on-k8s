// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"

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
// autoOpsMandatoryConfig.
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

// autoOpsMandatoryConfig is merged last so its values always take precedence over the
// baseline and any user-supplied config. SSL/hosts are not part of this config — they are
// injected per-module via injectModuleConnectionSettings.
const autoOpsMandatoryConfig = `
receivers:
  metricbeatreceiver:
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

const autoOpsDefaultModulesConfig = `
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        # Metrics
        - module: autoops_es
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
          period: 24h
          metricsets:
            - cat_template
            - component_template
            - index_template
`

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

// hasUserAutoOpsESModules reports whether any of the supplied configs defines at least one
// autoops_es module entry under receivers.metricbeatreceiver.metricbeat.modules. When true
// the operator skips injecting its own default modules so the user has full control over
// which autoops_es data is collected (e.g. to drop the Templates module).
// A user config that only adds non-autoops_es modules returns false and the operator still
// injects its own defaults alongside the user's entries.
func hasUserAutoOpsESModules(cfgs ...*settings.CanonicalConfig) bool {
	for _, cfg := range cfgs {
		if cfg == nil {
			continue
		}
		var raw map[string]any
		if err := cfg.Unpack(&raw); err != nil {
			continue
		}
		for _, m := range autoopsModules(raw) {
			mod, ok := m.(map[string]any)
			if ok && mod["module"] == "autoops_es" {
				return true
			}
		}
	}
	return false
}

// injectModuleConnectionSettings walks every autoops_es module entry in the merged config
// and injects the hosts and ssl fields. This is done post-merge because module entries may
// come from either the operator defaults or the user, while SSL paths are per-ES and only
// known at reconcile time.
func injectModuleConnectionSettings(cfg *settings.CanonicalConfig, sslEnabled bool, caCertPath, clientCertPath, clientKeyPath string) (*settings.CanonicalConfig, error) {
	var raw map[string]any
	if err := cfg.Unpack(&raw); err != nil {
		return nil, fmt.Errorf("while unpacking config: %w", err)
	}

	for _, m := range autoopsModules(raw) {
		mod, ok := m.(map[string]any)
		if !ok || mod["module"] != "autoops_es" {
			continue
		}
		mod["hosts"] = "${env:AUTOOPS_ES_URL}"
		ssl := map[string]any{}
		if sslEnabled {
			ssl["verification_mode"] = "certificate"
			ssl["certificate_authorities"] = []string{caCertPath}
			if clientCertPath != "" {
				ssl["certificate"] = clientCertPath
				ssl["key"] = clientKeyPath
			}
		} else {
			ssl["verification_mode"] = "none"
		}
		mod["ssl"] = ssl
	}

	return settings.NewCanonicalConfigFrom(raw)
}

// autoopsModules extracts the modules slice at receivers.metricbeatreceiver.metricbeat.modules.
// Returns nil if the path does not exist or is not a slice.
func autoopsModules(raw map[string]any) []any {
	receivers, ok := raw["receivers"].(map[string]any)
	if !ok {
		return nil
	}
	mbr, ok := receivers["metricbeatreceiver"].(map[string]any)
	if !ok {
		return nil
	}
	mb, ok := mbr["metricbeat"].(map[string]any)
	if !ok {
		return nil
	}
	modules, _ := mb["modules"].([]any)
	return modules
}

// buildAutoOpsESConfigMap builds the expected ConfigMap for autoops configuration.
// Merge order: baseline → userCfg → userSecretCfg → defaultModules → mandatory.
// The mandatory config is applied last so operator-owned scalars always take precedence.
// SSL and hosts are injected into every autoops_es module post-merge.
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

	mandatoryCfg, err := settings.ParseConfig([]byte(autoOpsMandatoryConfig))
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while parsing mandatory config: %w", err)
	}

	baselineCfg, err := settings.ParseConfig([]byte(autoOpsBaselineConfig))
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while parsing baseline config: %w", err)
	}

	// If neither user config source defines an autoops_es modules list, inject the operator
	// defaults so the agent collects the standard metric and template sets. When the user
	// does supply modules the operator's defaults are intentionally omitted, giving the user
	// full control over which data is collected.
	var defaultModulesCfg *settings.CanonicalConfig
	if !hasUserAutoOpsESModules(userCfg, userSecretCfg) {
		defaultModulesCfg, err = settings.ParseConfig([]byte(autoOpsDefaultModulesConfig))
		if err != nil {
			return corev1.ConfigMap{}, fmt.Errorf("while parsing default modules config: %w", err)
		}
	}

	// Merge order: baseline → user → userSecret → defaultModules → mandatory.
	// User config can override anything in the baseline (e.g. sending_queue tuning).
	// Mandatory config is rendered last so all operator-owned fields always win.
	if err := baselineCfg.MergeWith(userCfg, userSecretCfg, defaultModulesCfg, mandatoryCfg); err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while merging config: %w", err)
	}

	// Inject connection settings (hosts, SSL) into every autoops_es module in the merged
	// config. This is done post-merge because the modules may come from the operator defaults
	// or from the user, and the SSL paths are per-ES and only known at reconcile time.
	finalCfg, err := injectModuleConnectionSettings(baselineCfg, sslEnabled, caCertPath, clientCertPath, clientKeyPath)
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("while injecting module connection settings: %w", err)
	}

	rendered, err := finalCfg.Render()
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
