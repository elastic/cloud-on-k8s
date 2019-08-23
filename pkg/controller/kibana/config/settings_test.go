// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ucfg "github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
)

var defaultConfig = []byte(`
elasticsearch:
  hosts:
  - ""
  username: ""
  password: ""
  ssl:
    certificateAuthorities: /usr/share/kibana/config/elasticsearch-certs/tls.crt
    verificationMode: certificate
server:
  host: "0"
  name: ""
  ssl: 
    enabled: true
    key: /mnt/elastic-internal/http-certs/tls.key
    certificate: /mnt/elastic-internal/http-certs/tls.crt
xpack:
  monitoring:
    ui:
      container:
        elasticsearch:
          enabled: true
`)

func TestNewConfigSettings(t *testing.T) {
	type args struct {
		client k8s.Client
		kb     v1alpha1.Kibana
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "default config",
			args: args{
				kb: v1alpha1.Kibana{},
			},
			want: defaultConfig,
		},
		{
			name: "without TLS",
			args: args{
				kb: v1alpha1.Kibana{
					Spec: v1alpha1.KibanaSpec{
						HTTP: commonv1alpha1.HTTPConfig{
							TLS: commonv1alpha1.TLSOptions{
								SelfSignedCertificate: &commonv1alpha1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
				},
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig)
				require.NoError(t, err)
				removed, err := (*ucfg.Config)(cfg).Remove("server.ssl", -1, settings.Options...)
				require.True(t, removed)
				require.NoError(t, err)
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
			wantErr: false,
		},
		{
			name: "with user config",
			args: args{
				kb: v1alpha1.Kibana{
					Spec: v1alpha1.KibanaSpec{
						Config: &commonv1alpha1.Config{
							Data: map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			want: append(defaultConfig, []byte(`foo: bar`)...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewConfigSettings(tt.args.client, tt.args.kb)
			if tt.wantErr {
				require.NotNil(t, err)
			}
			require.NoError(t, err)

			// convert "got" into something comparable
			var gotCfg map[string]interface{}
			require.NoError(t, got.Unpack(&gotCfg))

			// convert "want" into something comparable
			cfg, err := uyaml.NewConfig(tt.want, commonv1alpha1.CfgOptions...)
			require.NoError(t, err)
			var wantCfg map[string]interface{}
			require.NoError(t, cfg.Unpack(&wantCfg))
			if diff := deep.Equal(wantCfg, gotCfg); diff != nil {
				t.Error(diff)
			}
		})
	}
}
