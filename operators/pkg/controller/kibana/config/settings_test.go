// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/stretchr/testify/assert"
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

			assert.Equal(t, wantCfg, gotCfg)
		})
	}
}
