// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

func TestIsFIPSEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  CanonicalConfig
		want bool
	}{
		{
			name: "fips enabled",
			cfg: CanonicalConfig{
				CanonicalConfig: common.MustCanonicalConfig(map[string]any{
					"xpack.security.fips_mode.enabled": "true",
				}),
			},
			want: true,
		},
		{
			name: "fips enabled as boolean",
			cfg: CanonicalConfig{
				CanonicalConfig: common.MustCanonicalConfig(map[string]any{
					"xpack.security.fips_mode.enabled": true,
				}),
			},
			want: true,
		},
		{
			name: "fips disabled",
			cfg: CanonicalConfig{
				CanonicalConfig: common.MustCanonicalConfig(map[string]any{
					"xpack.security.fips_mode.enabled": "false",
				}),
			},
			want: false,
		},
		{
			name: "fips setting missing",
			cfg: CanonicalConfig{
				CanonicalConfig: common.MustCanonicalConfig(map[string]any{}),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsFIPSEnabled(tt.cfg))
		})
	}
}

func TestAnyNodeSetFIPSEnabled(t *testing.T) {
	tests := []struct {
		name    string
		configs []CanonicalConfig
		want    bool
	}{
		{
			name: "mixed configs include fips enabled",
			configs: []CanonicalConfig{
				{
					CanonicalConfig: common.MustCanonicalConfig(map[string]any{
						"xpack.security.fips_mode.enabled": "false",
					}),
				},
				{
					CanonicalConfig: common.MustCanonicalConfig(map[string]any{
						"xpack.security.fips_mode.enabled": "true",
					}),
				},
			},
			want: true,
		},
		{
			name: "all configs non-fips",
			configs: []CanonicalConfig{
				{
					CanonicalConfig: common.MustCanonicalConfig(map[string]any{
						"xpack.security.fips_mode.enabled": "false",
					}),
				},
				{
					CanonicalConfig: common.MustCanonicalConfig(map[string]any{}),
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, AnyNodeSetFIPSEnabled(tt.configs))
		})
	}
}

func TestHasUserProvidedKeystorePassword(t *testing.T) {
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
		want        bool
	}{
		{
			name: "contains KEYSTORE_PASSWORD",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Env:  []corev1.EnvVar{{Name: "KEYSTORE_PASSWORD", Value: "ignored"}},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "contains ES_KEYSTORE_PASSPHRASE_FILE",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Env:  []corev1.EnvVar{{Name: "ES_KEYSTORE_PASSPHRASE_FILE", Value: "/tmp/passphrase"}},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "missing both keystore env vars",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Env:  []corev1.EnvVar{{Name: "SOME_OTHER_VAR", Value: "x"}},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "missing elasticsearch container",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sidecar",
							Env:  []corev1.EnvVar{{Name: "KEYSTORE_PASSWORD", Value: "ignored"}},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, HasUserProvidedKeystorePassword(tt.podTemplate))
		})
	}
}
