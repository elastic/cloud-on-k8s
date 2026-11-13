// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestRenderPluginsConfig(t *testing.T) {
	tests := []struct {
		name    string
		plugins *esv1.PluginsSpec
		want    string
	}{
		{
			name:    "no plugins",
			plugins: nil,
			want:    "",
		},
		{
			name:    "empty items",
			plugins: &esv1.PluginsSpec{Proxy: "proxy.corp:3128"},
			want:    "",
		},
		{
			name: "official plugins only",
			plugins: &esv1.PluginsSpec{Items: []esv1.Plugin{
				{ID: "analysis-icu"},
				{ID: "analysis-kuromoji"},
			}},
			want: `plugins:
- id: analysis-icu
- id: analysis-kuromoji
`,
		},
		{
			name: "custom plugin with location and proxy, order preserved",
			plugins: &esv1.PluginsSpec{
				Proxy: "proxy.corp:3128",
				Items: []esv1.Plugin{
					{ID: "my-plugin", Location: "https://repo.corp/my-plugin-9.2.0.zip"},
					{ID: "analysis-icu"},
				},
			},
			want: `plugins:
- id: my-plugin
  location: https://repo.corp/my-plugin-9.2.0.zip
- id: analysis-icu
proxy: proxy.corp:3128
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Plugins: tt.plugins}}
			got, err := RenderPluginsConfig(es)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func TestReconcileConfig_Plugins(t *testing.T) {
	ssetName := "sset"
	config := CanonicalConfig{common.MustCanonicalConfig(map[string]string{"a": "b"})}
	rendered, err := config.Render()
	require.NoError(t, err)
	withPlugins := &esv1.PluginsSpec{Items: []esv1.Plugin{{ID: "analysis-icu"}}}
	existingWithPlugins := corev1.Secret{
		Namespace: "ns",
		Name:      ConfigSecretName(ssetName),
		Data: map[string][]byte{
			ConfigFileName:        rendered,
			PluginsConfigFileName: []byte("plugins:\n- id: analysis-icu\n"),
		},
	}
	tests := []struct {
		name        string
		client      k8s.Client
		plugins     *esv1.PluginsSpec
		wantPlugins string
	}{
		{
			name:        "no plugins: no plugins key",
			client:      k8s.NewFakeClient(),
			plugins:     nil,
			wantPlugins: "",
		},
		{
			name:        "plugins: plugins key rendered",
			client:      k8s.NewFakeClient(),
			plugins:     withPlugins,
			wantPlugins: "plugins:\n- id: analysis-icu\n",
		},
		{
			name:        "plugins removed: plugins key removed",
			client:      k8s.NewFakeClient(&existingWithPlugins),
			plugins:     nil,
			wantPlugins: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				Namespace: "ns",
				Name:      "cluster",
				Spec:      esv1.ElasticsearchSpec{Plugins: tt.plugins},
			}
			require.NoError(t, ReconcileConfig(t.Context(), tt.client, es, ssetName, config, metadata.Metadata{}))
			secret, err := GetESConfigSecret(tt.client, es.Namespace, ssetName)
			require.NoError(t, err)
			require.Equal(t, rendered, secret.Data[ConfigFileName])
			require.Equal(t, tt.wantPlugins, string(secret.Data[PluginsConfigFileName]))
		})
	}
}
