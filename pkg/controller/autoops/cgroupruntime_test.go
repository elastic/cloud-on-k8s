// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"testing"

	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

func Test_buildAutoOpsESConfigMap_cgroupRuntime(t *testing.T) {
	// operatorDefaults is the cgroup_runtime extension as the operator configures it.
	operatorDefaults := map[string]any{
		"gomaxprocs": map[string]any{"enabled": false},
		"gomemlimit": map[string]any{"enabled": true, "ratio": 0.8},
	}

	tests := []struct {
		name           string
		version        string
		specConfigYAML []byte
		// configRefYAML is the content of the secret referenced by spec.configRef, if any.
		configRefYAML []byte
		// wantExtension is the expected extensions.cgroup_runtime, nil when it must be absent.
		wantExtension map[string]any
		// wantServiceExtensions is the expected service.extensions.
		wantServiceExtensions []any
	}{
		{
			name:                  "agent older than 9.4.0 - extension not referenced",
			version:               "9.3.9",
			wantServiceExtensions: []any{"healthcheckv2"},
		},
		{
			name:                  "agent 9.2 - extension not referenced",
			version:               "9.2.4",
			wantServiceExtensions: []any{"healthcheckv2"},
		},
		{
			name:                  "agent 9.4.0 - extension enabled with operator defaults",
			version:               "9.4.0",
			wantExtension:         operatorDefaults,
			wantServiceExtensions: []any{"healthcheckv2", "cgroup_runtime"},
		},
		{
			name:                  "agent 9.4.0 pre-release - extension enabled",
			version:               "9.4.0-SNAPSHOT",
			wantExtension:         operatorDefaults,
			wantServiceExtensions: []any{"healthcheckv2", "cgroup_runtime"},
		},
		{
			name:                  "later agent - extension enabled",
			version:               "10.0.0",
			wantExtension:         operatorDefaults,
			wantServiceExtensions: []any{"healthcheckv2", "cgroup_runtime"},
		},
		{
			name:    "user tunes the ratio - user value wins, other defaults kept",
			version: "9.4.9",
			specConfigYAML: []byte(`
extensions:
  cgroup_runtime:
    gomemlimit:
      ratio: 0.7
`),
			wantExtension: map[string]any{
				"gomaxprocs": map[string]any{"enabled": false},
				"gomemlimit": map[string]any{"enabled": true, "ratio": 0.7},
			},
			wantServiceExtensions: []any{"healthcheckv2", "cgroup_runtime"},
		},
		{
			name:    "user disables gomemlimit - user value wins",
			version: "9.4.9",
			specConfigYAML: []byte(`
extensions:
  cgroup_runtime:
    gomemlimit:
      enabled: false
`),
			wantExtension: map[string]any{
				"gomaxprocs": map[string]any{"enabled": false},
				"gomemlimit": map[string]any{"enabled": false, "ratio": 0.8},
			},
			wantServiceExtensions: []any{"healthcheckv2", "cgroup_runtime"},
		},
		{
			name:    "user tunes the ratio in the configRef secret - user value wins",
			version: "9.4.9",
			configRefYAML: []byte(`
extensions:
  cgroup_runtime:
    gomemlimit:
      ratio: 0.6
`),
			wantExtension: map[string]any{
				"gomaxprocs": map[string]any{"enabled": false},
				"gomemlimit": map[string]any{"enabled": true, "ratio": 0.6},
			},
			wantServiceExtensions: []any{"healthcheckv2", "cgroup_runtime"},
		},
		{
			name:    "user already lists the extension - listed once",
			version: "9.4.9",
			specConfigYAML: []byte(`
extensions:
  cgroup_runtime:
    gomaxprocs:
      enabled: true
service:
  extensions: [cgroup_runtime]
`),
			wantExtension: map[string]any{
				"gomaxprocs": map[string]any{"enabled": true},
				"gomemlimit": map[string]any{"enabled": true, "ratio": 0.8},
			},
			wantServiceExtensions: []any{"cgroup_runtime", "healthcheckv2"},
		},
		{
			name:    "user configures the deprecated cgroupruntime type - operator adds nothing",
			version: "9.4.9",
			specConfigYAML: []byte(`
extensions:
  cgroupruntime:
    gomemlimit:
      ratio: 0.5
service:
  extensions: [cgroupruntime]
`),
			wantServiceExtensions: []any{"cgroupruntime", "healthcheckv2"},
		},
		{
			name:    "user configures a named instance - operator adds nothing",
			version: "9.4.9",
			specConfigYAML: []byte(`
extensions:
  cgroup_runtime/custom:
    gomemlimit:
      ratio: 0.5
service:
  extensions: [cgroup_runtime/custom]
`),
			wantServiceExtensions: []any{"cgroup_runtime/custom", "healthcheckv2"},
		},
		{
			name:    "user configures a named instance in the configRef secret - operator adds nothing",
			version: "9.4.9",
			configRefYAML: []byte(`
extensions:
  cgroup_runtime/custom: {}
service:
  extensions: [cgroup_runtime/custom]
`),
			wantServiceExtensions: []any{"cgroup_runtime/custom", "healthcheckv2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := mkPolicy()
			policy.Spec.Version = tt.version
			if len(tt.specConfigYAML) > 0 {
				parsed, err := uyaml.NewConfig(tt.specConfigYAML, commonv1.CfgOptions...)
				require.NoError(t, err)
				var data map[string]any
				require.NoError(t, parsed.Unpack(&data))
				policy.Spec.Config = &commonv1.Config{Data: data}
			}
			var secrets []client.Object
			if len(tt.configRefYAML) > 0 {
				policy.Spec.ConfigRef = &commonv1.ConfigSource{SecretName: "my-config-secret"}
				secrets = append(secrets, &corev1.Secret{
					Name: "my-config-secret", Namespace: policy.Namespace,
					Data: map[string][]byte{autoopsv1alpha1.ConfigFileName: tt.configRefYAML},
				})
			}

			r := mkReconciler(t, secrets...)
			cm, err := r.ReconcileAutoOpsESConfigMap(context.Background(), policy, mkES(false))
			require.NoError(t, err)
			require.NotNil(t, cm)

			got := unpackConfigYAML(t, *cm)
			extension, found := asMap(t, got["extensions"])[cgroupRuntimeExtension]
			if tt.wantExtension == nil {
				assert.False(t, found, "extensions.cgroup_runtime must not be set")
			} else {
				require.True(t, found, "extensions.cgroup_runtime must be set")
				assert.Equal(t, tt.wantExtension, extension)
			}
			assert.Equal(t, tt.wantServiceExtensions, asMap(t, got["service"])["extensions"])
		})
	}
}

func Test_supportsCgroupRuntime(t *testing.T) {
	for _, tt := range []struct {
		version string
		want    bool
	}{
		{version: "8.19.0", want: false},
		{version: "9.2.4", want: false},
		{version: "9.3.99", want: false},
		{version: "9.4.0-SNAPSHOT", want: true},
		{version: "9.4.0", want: true},
		{version: "9.4.9", want: true},
		{version: "9.5.0", want: true},
		{version: "10.0.0", want: true},
		{version: "not-a-version", want: false},
	} {
		t.Run(tt.version, func(t *testing.T) {
			policy := mkPolicy()
			policy.Spec.Version = tt.version
			assert.Equal(t, tt.want, supportsCgroupRuntime(policy))
		})
	}
}
