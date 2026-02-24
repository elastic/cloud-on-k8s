// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

var defaultConfigNoSSL = []byte(`
exporters:
  otlphttp:
    endpoint: ${env:AUTOOPS_OTEL_URL}
    headers:
      Authorization: AutoOpsToken ${env:AUTOOPS_TOKEN}
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
			HTTP: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled: !sslEnabled,
					},
				},
			},
		},
	}
}

func Test_buildAutoOpsESConfigMap(t *testing.T) {
	type args struct {
		policy func() autoopsv1alpha1.AutoOpsAgentPolicy
		es     func() esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "default config without SSL",
			args: args{
				policy: mkPolicy,
				es:     func() esv1.Elasticsearch { return mkES(false) },
			},
			want: defaultConfigNoSSL,
		},
		{
			name: "default config with SSL",
			args: args{
				policy: mkPolicy,
				es:     func() esv1.Elasticsearch { return mkES(true) },
			},
			want: defaultConfigWithSSL,
		},
		{
			name: "with metadata labels and annotations",
			args: args{
				policy: func() autoopsv1alpha1.AutoOpsAgentPolicy {
					p := mkPolicy()
					p.ObjectMeta.Labels = map[string]string{
						"label1": "value1",
						"label2": "value2",
					}
					p.ObjectMeta.Annotations = map[string]string{
						"annotation1": "value1",
					}
					return p
				},
				es: func() esv1.Elasticsearch {
					es := mkES(false)
					es.Namespace = "test-namespace"
					return es
				},
			},
			want: defaultConfigNoSSL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAutoOpsESConfigMap(tt.args.policy(), tt.args.es())
			if (err != nil) != tt.wantErr {
				t.Errorf("buildAutoOpsESConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				policy := tt.args.policy()
				es := tt.args.es()
				// Validate ConfigMap structure
				expectedName := autoopsv1alpha1.Config(policy.GetName(), es)
				if got.Name != expectedName {
					t.Errorf("buildAutoOpsESConfigMap() ConfigMap name = %v, want %v", got.Name, expectedName)
				}
				if got.Namespace != policy.GetNamespace() {
					t.Errorf("buildAutoOpsESConfigMap() ConfigMap namespace = %v, want %v", got.Namespace, policy.GetNamespace())
				}
				configYAML, exists := got.Data[autoOpsESConfigFileName]
				if !exists {
					t.Errorf("buildAutoOpsESConfigMap() ConfigMap missing key %v", autoOpsESConfigFileName)
					return
				}

				// Parse both configs for comparison
				var gotCfg map[string]any
				gotParsed, err := uyaml.NewConfig([]byte(configYAML), commonv1.CfgOptions...)
				require.NoError(t, err)
				require.NoError(t, gotParsed.Unpack(&gotCfg))

				var wantCfg map[string]any
				wantParsed, err := uyaml.NewConfig(tt.want, commonv1.CfgOptions...)
				require.NoError(t, err)
				require.NoError(t, wantParsed.Unpack(&wantCfg))

				if diff := deep.Equal(wantCfg, gotCfg); diff != nil {
					t.Errorf("buildAutoOpsESConfigMap() config mismatch: %v", diff)
				}
			}
		})
	}
}
