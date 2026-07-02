// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"testing"

	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var defaultConfigNoSSL = []byte(`
exporters:
  otlphttp:
    endpoint: ${env:AUTOOPS_OTEL_URL}
    headers:
      Authorization: AutoOpsToken ${env:AUTOOPS_TOKEN}
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
extensions:
  healthcheckv2:
    component_health:
      include_permanent_errors: true
      include_recoverable_errors: true
      recovery_duration: 5m
    http:
      config:
        enabled: true
        path: /health/config
      endpoint: 0.0.0.0:13133
      status:
        enabled: true
        path: /health/status
    use_v2: true
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - hosts: ${env:AUTOOPS_ES_URL}
          metricsets:
            - cat_shards
            - cluster_health
            - cluster_settings
            - license
            - node_stats
            - tasks_management
          module: autoops_es
          period: 10s
          ssl:
            verification_mode: none
        - hosts: ${env:AUTOOPS_ES_URL}
          metricsets:
            - cat_template
            - component_template
            - index_template
          module: autoops_es
          period: 24h
          ssl:
            verification_mode: none
    output:
      otelconsumer: null
    processors:
      - add_fields:
          fields:
            token: ${env:AUTOOPS_TOKEN}
          target: autoops_es
    telemetry_types:
      - logs
service:
  extensions:
    - healthcheckv2
  pipelines:
    logs:
      exporters:
        - otlphttp
      receivers:
        - metricbeatreceiver
  telemetry:
    logs:
      encoding: json
`)

var defaultConfigWithSSL = []byte(`
exporters:
  otlphttp:
    endpoint: ${env:AUTOOPS_OTEL_URL}
    headers:
      Authorization: AutoOpsToken ${env:AUTOOPS_TOKEN}
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
extensions:
  healthcheckv2:
    component_health:
      include_permanent_errors: true
      include_recoverable_errors: true
      recovery_duration: 5m
    http:
      config:
        enabled: true
        path: /health/config
      endpoint: 0.0.0.0:13133
      status:
        enabled: true
        path: /health/status
    use_v2: true
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - hosts: ${env:AUTOOPS_ES_URL}
          metricsets:
            - cat_shards
            - cluster_health
            - cluster_settings
            - license
            - node_stats
            - tasks_management
          module: autoops_es
          period: 10s
          ssl:
            certificate_authorities:
              - /mnt/elastic-internal/es-ca/default-test-es/ca.crt
            verification_mode: certificate
        - hosts: ${env:AUTOOPS_ES_URL}
          metricsets:
            - cat_template
            - component_template
            - index_template
          module: autoops_es
          period: 24h
          ssl:
            certificate_authorities:
              - /mnt/elastic-internal/es-ca/default-test-es/ca.crt
            verification_mode: certificate
    output:
      otelconsumer: null
    processors:
      - add_fields:
          fields:
            token: ${env:AUTOOPS_TOKEN}
          target: autoops_es
    telemetry_types:
      - logs
service:
  extensions:
    - healthcheckv2
  pipelines:
    logs:
      exporters:
        - otlphttp
      receivers:
        - metricbeatreceiver
  telemetry:
    logs:
      encoding: json
`)

var defaultConfigWithSSLAndClientCert = []byte(`
exporters:
  otlphttp:
    endpoint: ${env:AUTOOPS_OTEL_URL}
    headers:
      Authorization: AutoOpsToken ${env:AUTOOPS_TOKEN}
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
extensions:
  healthcheckv2:
    component_health:
      include_permanent_errors: true
      include_recoverable_errors: true
      recovery_duration: 5m
    http:
      config:
        enabled: true
        path: /health/config
      endpoint: 0.0.0.0:13133
      status:
        enabled: true
        path: /health/status
    use_v2: true
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - hosts: ${env:AUTOOPS_ES_URL}
          metricsets:
            - cat_shards
            - cluster_health
            - cluster_settings
            - license
            - node_stats
            - tasks_management
          module: autoops_es
          period: 10s
          ssl:
            certificate: /mnt/elastic-internal/es-client-cert/default-test-es/tls.crt
            certificate_authorities:
              - /mnt/elastic-internal/es-ca/default-test-es/ca.crt
            key: /mnt/elastic-internal/es-client-cert/default-test-es/tls.key
            verification_mode: certificate
        - hosts: ${env:AUTOOPS_ES_URL}
          metricsets:
            - cat_template
            - component_template
            - index_template
          module: autoops_es
          period: 24h
          ssl:
            certificate: /mnt/elastic-internal/es-client-cert/default-test-es/tls.crt
            certificate_authorities:
              - /mnt/elastic-internal/es-ca/default-test-es/ca.crt
            key: /mnt/elastic-internal/es-client-cert/default-test-es/tls.key
            verification_mode: certificate
    output:
      otelconsumer: null
    processors:
      - add_fields:
          fields:
            token: ${env:AUTOOPS_TOKEN}
          target: autoops_es
    telemetry_types:
      - logs
service:
  extensions:
    - healthcheckv2
  pipelines:
    logs:
      exporters:
        - otlphttp
      receivers:
        - metricbeatreceiver
  telemetry:
    logs:
      encoding: json
`)

func mkPolicy() autoopsv1alpha1.AutoOpsAgentPolicy {
	return autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version: "9.2.4",
		},
	}
}

func mkES(sslEnabled bool) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es",
			Namespace: "default",
		},
		Spec: esv1.ElasticsearchSpec{
			HTTP: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					TLSOptions: commonv1.TLSOptions{
						SelfSignedCertificate: &commonv1.SelfSignedCertificate{
							Disabled: !sslEnabled,
						},
					},
				},
			},
		},
	}
}

// mkReconciler builds a minimal AgentPolicyReconciler for use in tests.
func mkReconciler(t *testing.T, objects ...client.Object) *AgentPolicyReconciler {
	t.Helper()
	scheme.SetupScheme()
	c := k8s.NewFakeClient(objects...)
	return &AgentPolicyReconciler{
		Client:         c,
		dynamicWatches: watches.NewDynamicWatches(),
		recorder:       toolsevents.NewFakeRecorder(10),
	}
}

// asMap asserts v is map[string]any, failing the test immediately if not.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", v)
	return m
}

// asSlice asserts v is []any, failing the test immediately if not.
func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	require.True(t, ok, "expected []any, got %T", v)
	return s
}

// unpackConfigYAML parses a YAML string from a ConfigMap into a map for assertions.
func unpackConfigYAML(t *testing.T, cm corev1.ConfigMap) map[string]any {
	t.Helper()
	data, ok := cm.Data[autoopsv1alpha1.ConfigFileName]
	require.True(t, ok, "ConfigMap missing key %s", autoopsv1alpha1.ConfigFileName)
	var out map[string]any
	parsed, err := uyaml.NewConfig([]byte(data), commonv1.CfgOptions...)
	require.NoError(t, err)
	require.NoError(t, parsed.Unpack(&out))
	return out
}

func Test_buildAutoOpsESConfigMap(t *testing.T) {
	// validConfigRefYAML is a minimal valid config used by ConfigRef test cases.
	validConfigRefYAML := []byte(`
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - module: autoops_es
          hosts: ${env:AUTOOPS_ES_URL}
          period: 10s
          ssl:
            verification_mode: none
          metricsets:
            - cluster_health
`)

	tests := []struct {
		name           string
		policy         func() autoopsv1alpha1.AutoOpsAgentPolicy // defaults to mkPolicy()
		es             func() esv1.Elasticsearch                 // defaults to mkES(false)
		specConfigYAML []byte                                    // raw YAML parsed into policy.Spec.Config
		secrets        []client.Object
		want           []byte                                  // exact YAML comparison via ucfg parse
		check          func(t *testing.T, cm corev1.ConfigMap) // custom assertions
		wantErr        bool
	}{
		{
			name: "default config without SSL",
			want: defaultConfigNoSSL,
		},
		{
			name: "default config with SSL",
			es:   func() esv1.Elasticsearch { return mkES(true) },
			want: defaultConfigWithSSL,
		},
		{
			name: "default config with SSL and client cert",
			es: func() esv1.Elasticsearch {
				es := mkES(true)
				es.Annotations = map[string]string{annotation.ClientAuthenticationRequiredAnnotation: "true"}
				return es
			},
			want: defaultConfigWithSSLAndClientCert,
		},
		{
			name: "client auth annotation with SSL disabled does not render client cert paths",
			es: func() esv1.Elasticsearch {
				es := mkES(false)
				es.Annotations = map[string]string{annotation.ClientAuthenticationRequiredAnnotation: "true"}
				return es
			},
			want: defaultConfigNoSSL,
		},
		{
			name: "metadata labels and annotations propagated to ConfigMap",
			policy: func() autoopsv1alpha1.AutoOpsAgentPolicy {
				p := mkPolicy()
				p.Labels = map[string]string{"label1": "value1", "label2": "value2"}
				p.Annotations = map[string]string{"annotation1": "value1"}
				return p
			},
			es: func() esv1.Elasticsearch {
				es := mkES(false)
				es.Namespace = "test-namespace"
				return es
			},
			want: defaultConfigNoSSL,
		},
		{
			name: "nil user config - output is identical to default",
			want: defaultConfigNoSSL,
		},
		{
			name: "user adds extra OTLP header - preserved alongside operator headers",
			specConfigYAML: []byte(`
exporters:
  otlphttp:
    headers:
      X-Custom-Header: my-value
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				headers := asMap(t, asMap(t, asMap(t, got["exporters"])["otlphttp"])["headers"])
				assert.Equal(t, "my-value", headers["X-Custom-Header"])
				assert.Equal(t, "AutoOpsToken ${env:AUTOOPS_TOKEN}", headers["Authorization"])
			},
		},
		{
			// endpoint is in autoOpsMandatoryConfig, so it is re-applied after user config.
			name: "user overrides endpoint - mandatory config wins",
			specConfigYAML: []byte(`
exporters:
  otlphttp:
    endpoint: https://custom.example.com
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				otlp := asMap(t, asMap(t, unpackConfigYAML(t, cm)["exporters"])["otlphttp"])
				assert.Equal(t, "${env:AUTOOPS_OTEL_URL}", otlp["endpoint"])
			},
		},
		{
			name: "user increases sending_queue size - user value wins",
			specConfigYAML: []byte(`
exporters:
  otlphttp:
    sending_queue:
      queue_size: 104857600
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				sq := asMap(t, asMap(t, asMap(t, got["exporters"])["otlphttp"])["sending_queue"])
				assert.EqualValues(t, 104857600, sq["queue_size"], "user-supplied queue_size must win over the default 52428800")
				// Other sending_queue fields from the operator default must still be present.
				assert.Equal(t, true, sq["block_on_overflow"])
				assert.Equal(t, true, sq["enabled"])
			},
		},
		{
			// Even an explicit empty string is overridden by autoOpsMandatoryConfig.
			name: "user clears otlphttp endpoint - mandatory config re-applies env var reference",
			specConfigYAML: []byte(`
exporters:
  otlphttp:
    endpoint: ""
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				otlp := asMap(t, asMap(t, unpackConfigYAML(t, cm)["exporters"])["otlphttp"])
				assert.Equal(t, "${env:AUTOOPS_OTEL_URL}", otlp["endpoint"])
			},
		},
		{
			// AppendValues appends to existing lists: [] appends nothing, so healthcheckv2 stays.
			name: "user provides empty service.extensions - healthcheckv2 preserved by AppendValues",
			specConfigYAML: []byte(`
service:
  extensions: []
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				exts := asSlice(t, asMap(t, unpackConfigYAML(t, cm)["service"])["extensions"])
				found := false
				for _, e := range exts {
					if e == "healthcheckv2" {
						found = true
					}
				}
				assert.True(t, found, "healthcheckv2 must still be present")
			},
		},
		{
			// ucfg always deep-merges maps regardless of AppendValues: receivers: {} deep-merges
			// with the existing receivers map, preserving metricbeatreceiver and all its content.
			name: "user provides empty receivers map - original receivers preserved by map merge",
			specConfigYAML: []byte(`
receivers: {}
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				receivers, ok := got["receivers"].(map[string]any)
				require.True(t, ok, "receivers section must be present")
				mbr, ok := receivers["metricbeatreceiver"].(map[string]any)
				require.True(t, ok, "metricbeatreceiver must be present")
				modules, ok := mbr["metricbeat"].(map[string]any)["modules"].([]any)
				require.True(t, ok, "modules must be present")
				found := false
				for _, m := range modules {
					if mod, ok := m.(map[string]any); ok && mod["module"] == "autoops_es" {
						found = true
					}
				}
				assert.True(t, found, "autoops_es module must survive the empty-map merge")
			},
		},
		{
			// healthcheckv2.http.endpoint is in autoOpsMandatoryConfig and is always re-applied.
			name: "user changes healthcheckv2 endpoint port - mandatory config re-applies correct port",
			specConfigYAML: []byte(`
extensions:
  healthcheckv2:
    http:
      endpoint: "0.0.0.0:9999"
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				hc := asMap(t, asMap(t, got["extensions"])["healthcheckv2"])
				assert.Equal(t, "0.0.0.0:13133", asMap(t, hc["http"])["endpoint"])
			},
		},
		{
			// service.telemetry.logs.encoding is in autoOpsMandatoryConfig, so it is
			// always re-applied after user config.
			name: "user sets service.telemetry.logs.encoding - mandatory config wins",
			specConfigYAML: []byte(`
service:
  telemetry:
    logs:
      encoding: text
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				logs := asMap(t, asMap(t, asMap(t, got["service"])["telemetry"])["logs"])
				assert.Equal(t, "json", logs["encoding"])
			},
		},
		{
			// AppendValues appends to existing lists. The user's module comes first (from the
			// user config layer), and the operator's two autoops_es modules are appended after.
			name: "user adds non-autoops_es module - appended after operator modules",
			specConfigYAML: []byte(`
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - module: some_other_module
          period: 10s
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				modules := asSlice(t, asMap(t, asMap(t, asMap(t, got["receivers"])["metricbeatreceiver"])["metricbeat"])["modules"])
				foundAutoOps, foundOther := false, false
				for _, m := range modules {
					if mod, ok := m.(map[string]any); ok {
						switch mod["module"] {
						case "autoops_es":
							foundAutoOps = true
						case "some_other_module":
							foundOther = true
						}
					}
				}
				assert.True(t, foundAutoOps, "operator autoops_es modules must be preserved")
				assert.True(t, foundOther, "user module must be appended")
			},
		},
		{
			// When the user provides an autoops_es module the operator skips injecting its own
			// default modules (Metrics + Templates), giving the user full control over which
			// autoops_es data is collected. The operator still injects connection settings
			// (hosts, ssl) into all autoops_es modules and always merges mandatory fields last.
			// Non-module user additions (exporters, pipelines) are present alongside the
			// operator's mandatory ones.
			name: "user provides autoops_es module - replaces operator defaults, custom exporter and pipeline added alongside",
			specConfigYAML: []byte(`
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - module: autoops_es
          period: 30s
          metricsets:
            - cluster_health
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    logs/debug:
      receivers: [metricbeatreceiver]
      exporters: [debug]
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)

				// Only the user's module is present — operator defaults are not injected when
				// the user supplies at least one autoops_es module.
				modules := asSlice(t, asMap(t, asMap(t, asMap(t, got["receivers"])["metricbeatreceiver"])["metricbeat"])["modules"])
				require.Len(t, modules, 1, "only the user's autoops_es module, operator defaults omitted")

				mod := asMap(t, modules[0])
				assert.Equal(t, "autoops_es", mod["module"])
				assert.Equal(t, "30s", mod["period"])
				// Operator injects connection settings regardless of module source.
				assert.Equal(t, "${env:AUTOOPS_ES_URL}", mod["hosts"])
				assert.Equal(t, "none", asMap(t, mod["ssl"])["verification_mode"])

				// User's exporter is present alongside the operator's otlphttp.
				exporters := asMap(t, got["exporters"])
				_, hasDebug := exporters["debug"]
				_, hasOTLP := exporters["otlphttp"]
				assert.True(t, hasDebug, "user's debug exporter must be present")
				assert.True(t, hasOTLP, "operator's otlphttp exporter must be present")

				// User's custom pipeline is present alongside the mandatory logs pipeline.
				pipelines := asMap(t, asMap(t, got["service"])["pipelines"])
				_, hasLogs := pipelines["logs"]
				_, hasDebugPipeline := pipelines["logs/debug"]
				assert.True(t, hasLogs, "mandatory logs pipeline must be present")
				assert.True(t, hasDebugPipeline, "user's logs/debug pipeline must be present")
			},
		},
		{
			// When the user provides autoops_es modules and SSL is enabled on the ES cluster,
			// injectModuleConnectionSettings must always override any user-supplied ssl fields
			// with the operator-managed values. The user-supplied ssl.verification_mode: none
			// and a bogus CA path must be replaced by the operator's certificate settings.
			name: "user provides autoops_es module with SSL-enabled ES - operator always overrides ssl fields",
			es:   func() esv1.Elasticsearch { return mkES(true) },
			specConfigYAML: []byte(`
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - module: autoops_es
          period: 30s
          metricsets:
            - cluster_health
          hosts: http://wrong-host:9200
          ssl:
            verification_mode: none
            certificate_authorities:
              - /wrong/ca/path.crt
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				modules := asSlice(t, asMap(t, asMap(t, asMap(t, got["receivers"])["metricbeatreceiver"])["metricbeat"])["modules"])
				require.Len(t, modules, 1, "only the user's module, operator defaults omitted")

				mod := asMap(t, modules[0])
				assert.Equal(t, "${env:AUTOOPS_ES_URL}", mod["hosts"], "operator must override user-supplied hosts")
				ssl := asMap(t, mod["ssl"])
				assert.Equal(t, "certificate", ssl["verification_mode"], "operator must override user-supplied verification_mode")
				cas := asSlice(t, ssl["certificate_authorities"])
				require.Len(t, cas, 1)
				assert.Equal(t, "/mnt/elastic-internal/es-ca/default-test-es/ca.crt", cas[0], "operator must override user-supplied CA path")
				_, hasClientCert := ssl["certificate"]
				assert.False(t, hasClientCert, "no client cert expected when client auth is not required")
			},
		},
		{
			// When client authentication is required the operator must inject ssl.certificate
			// and ssl.key, overriding any user-supplied values for all ssl connection fields.
			name: "user provides autoops_es module with SSL and client cert - operator always overrides all ssl fields",
			es: func() esv1.Elasticsearch {
				es := mkES(true)
				es.Annotations = map[string]string{annotation.ClientAuthenticationRequiredAnnotation: "true"}
				return es
			},
			specConfigYAML: []byte(`
receivers:
  metricbeatreceiver:
    metricbeat:
      modules:
        - module: autoops_es
          period: 30s
          metricsets:
            - cluster_health
          hosts: http://wrong-host:9200
          ssl:
            verification_mode: none
            certificate_authorities:
              - /wrong/ca/path.crt
            certificate: /wrong/cert.crt
            key: /wrong/key.key
`),
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				modules := asSlice(t, asMap(t, asMap(t, asMap(t, got["receivers"])["metricbeatreceiver"])["metricbeat"])["modules"])
				require.Len(t, modules, 1)

				mod := asMap(t, modules[0])
				assert.Equal(t, "${env:AUTOOPS_ES_URL}", mod["hosts"], "operator must override user-supplied hosts")
				ssl := asMap(t, mod["ssl"])
				assert.Equal(t, "certificate", ssl["verification_mode"], "operator must override user-supplied verification_mode")
				cas := asSlice(t, ssl["certificate_authorities"])
				require.Len(t, cas, 1)
				assert.Equal(t, "/mnt/elastic-internal/es-ca/default-test-es/ca.crt", cas[0], "operator must override user-supplied CA path")
				assert.Equal(t, "/mnt/elastic-internal/es-client-cert/default-test-es/tls.crt", ssl["certificate"], "operator must override user-supplied certificate path")
				assert.Equal(t, "/mnt/elastic-internal/es-client-cert/default-test-es/tls.key", ssl["key"], "operator must override user-supplied key path")
			},
		},
		// ConfigRef: secret contains valid config - autoops_es module present after merge.
		{
			name: "ConfigRef secret with valid config - merged successfully",
			policy: func() autoopsv1alpha1.AutoOpsAgentPolicy {
				p := mkPolicy()
				p.Spec.ConfigRef = &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-config-secret"}}
				return p
			},
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "my-config-secret", Namespace: "default"},
					Data:       map[string][]byte{autoopsv1alpha1.ConfigFileName: validConfigRefYAML},
				},
			},
			check: func(t *testing.T, cm corev1.ConfigMap) {
				t.Helper()
				got := unpackConfigYAML(t, cm)
				modules := asSlice(t, asMap(t, asMap(t, asMap(t, got["receivers"])["metricbeatreceiver"])["metricbeat"])["modules"])
				var autoOpsModule map[string]any
				for _, m := range modules {
					if mod, ok := m.(map[string]any); ok && mod["module"] == "autoops_es" {
						autoOpsModule = mod
						break
					}
				}
				require.NotNil(t, autoOpsModule, "autoops_es module must be present")
				assert.Equal(t, "${env:AUTOOPS_ES_URL}", autoOpsModule["hosts"], "operator must override user-supplied hosts")
				ssl := asMap(t, autoOpsModule["ssl"])
				assert.Equal(t, "none", ssl["verification_mode"], "operator must set verification_mode to none for non-SSL ES")
			},
		},
		{
			name: "ConfigRef secret missing expected key - error",
			policy: func() autoopsv1alpha1.AutoOpsAgentPolicy {
				p := mkPolicy()
				p.Spec.ConfigRef = &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "my-config-secret"}}
				return p
			},
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "my-config-secret", Namespace: "default"},
					Data:       map[string][]byte{"wrong-key.yml": validConfigRefYAML},
				},
			},
			wantErr: true,
		},
		{
			name: "ConfigRef secret not found - error",
			policy: func() autoopsv1alpha1.AutoOpsAgentPolicy {
				p := mkPolicy()
				p.Spec.ConfigRef = &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: "nonexistent-secret"}}
				return p
			},
			secrets: []client.Object{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := mkPolicy()
			if tt.policy != nil {
				policy = tt.policy()
			}
			es := mkES(false)
			if tt.es != nil {
				es = tt.es()
			}

			if len(tt.specConfigYAML) > 0 {
				parsed, err := uyaml.NewConfig(tt.specConfigYAML, commonv1.CfgOptions...)
				require.NoError(t, err)
				var data map[string]any
				require.NoError(t, parsed.Unpack(&data))
				policy.Spec.Config = &commonv1.Config{Data: data}
			}

			r := mkReconciler(t, tt.secrets...)
			cm, err := r.ReconcileAutoOpsESConfigMap(context.Background(), policy, es)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cm)

			// Always validate ConfigMap identity.
			assert.Equal(t, autoopsv1alpha1.Config(policy.GetName(), es), cm.Name)
			assert.Equal(t, policy.GetNamespace(), cm.Namespace)
			_, hasKey := cm.Data[autoopsv1alpha1.ConfigFileName]
			assert.True(t, hasKey, "ConfigMap must contain key %s", autoopsv1alpha1.ConfigFileName)

			if tt.want != nil {
				var gotCfg, wantCfg map[string]any
				gotParsed, err := uyaml.NewConfig([]byte(cm.Data[autoopsv1alpha1.ConfigFileName]), commonv1.CfgOptions...)
				require.NoError(t, err)
				require.NoError(t, gotParsed.Unpack(&gotCfg))
				wantParsed, err := uyaml.NewConfig(tt.want, commonv1.CfgOptions...)
				require.NoError(t, err)
				require.NoError(t, wantParsed.Unpack(&wantCfg))
				if diff := deep.Equal(wantCfg, gotCfg); diff != nil {
					t.Errorf("config mismatch: %v", diff)
				}
			}
			if tt.check != nil {
				tt.check(t, *cm)
			}
		})
	}
}
