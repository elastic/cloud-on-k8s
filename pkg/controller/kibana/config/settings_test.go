// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ucfg "github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var defaultConfig = []byte(`
elasticsearch:
  hosts:
    - ""
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

var associationConfig = []byte(`
elasticsearch:
  hosts:
    - "https://es-url:9200"
  username: "elastic"
  password: "password"
  ssl:
    certificateAuthorities: /usr/share/kibana/config/elasticsearch-certs/ca.crt
    verificationMode: certificate
`)

func TestNewConfigSettings(t *testing.T) {
	type args struct {
		client k8s.Client
		kb     func() kbv1.Kibana
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
				kb: mkKibana,
			},
			want: defaultConfig,
		},
		{
			name: "without TLS",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec = kbv1.KibanaSpec{
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					}
					return kb
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
			name: "with Association",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec = kbv1.KibanaSpec{
						ElasticsearchRef: commonv1.ObjectSelector{Name: "test-es"},
					}
					kb.SetAssociationConf(&commonv1.AssociationConf{
						AuthSecretName: "auth-secret",
						AuthSecretKey:  "elastic",
						CASecretName:   "ca-secret",
						CACertProvided: true,
						URL:            "https://es-url:9200",
					})
					return kb
				},
				client: k8s.WrapClient(fake.NewFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "auth-secret",
						},
						Data: map[string][]byte{
							"elastic": []byte("password"),
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ca-secret",
						},
						Data: map[string][]byte{
							"ca.crt": []byte("certificate"),
						},
					},
				)),
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig)
				require.NoError(t, err)
				removed, err := (*ucfg.Config)(cfg).Remove("elasticsearch.hosts", -1, settings.Options...)
				require.True(t, removed)
				require.NoError(t, err)
				assocCfg, err := settings.ParseConfig(associationConfig)
				require.NoError(t, err)
				require.NoError(t, cfg.MergeWith(assocCfg))
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
			wantErr: false,
		},
		{
			name: "with user config",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec = kbv1.KibanaSpec{
						Config: &commonv1.Config{
							Data: map[string]interface{}{
								"foo": "bar",
							},
						},
					}
					return kb
				},
			},
			want: append(defaultConfig, []byte(`foo: bar`)...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versionSpecificCfg := settings.MustCanonicalConfig(map[string]interface{}{"elasticsearch.hosts": nil})
			got, err := NewConfigSettings(tt.args.client, tt.args.kb(), versionSpecificCfg)
			if tt.wantErr {
				require.NotNil(t, err)
			}
			require.NoError(t, err)

			// convert "got" into something comparable
			var gotCfg map[string]interface{}
			require.NoError(t, got.Unpack(&gotCfg))

			// convert "want" into something comparable
			cfg, err := uyaml.NewConfig(tt.want, commonv1.CfgOptions...)
			require.NoError(t, err)
			var wantCfg map[string]interface{}
			require.NoError(t, cfg.Unpack(&wantCfg))
			if diff := deep.Equal(wantCfg, gotCfg); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func mkKibana() kbv1.Kibana {
	kb := kbv1.Kibana{}
	return kb
}

func Test_getExistingConfig(t *testing.T) {

	testKb := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testkb",
			Namespace: "testns",
		},
	}
	testValidSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(testKb),
			Namespace: testKb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: defaultConfig,
		},
	}
	testNoYaml := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(testKb),
			Namespace: testKb.Namespace,
		},
		Data: map[string][]byte{
			"notarealkey": []byte(`:-{`),
		},
	}
	testInvalidYaml := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(testKb),
			Namespace: testKb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: []byte(`:-{`),
		},
	}

	tests := []struct {
		name      string
		kb        kbv1.Kibana
		secret    corev1.Secret
		expectErr bool
		expectKey string
	}{
		{
			name:      "happy path",
			kb:        testKb,
			secret:    testValidSecret,
			expectErr: false,
			expectKey: "xpack",
		},
		{
			name:      "no secret exists",
			kb:        testKb,
			secret:    corev1.Secret{},
			expectErr: true,
			expectKey: "",
		},
		{
			name:      "no kibana.yml exists in secret",
			kb:        testKb,
			secret:    testNoYaml,
			expectErr: true,
			expectKey: "",
		},
		{
			name:      "cannot parse yaml",
			kb:        testKb,
			secret:    testInvalidYaml,
			expectErr: true,
			expectKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(&tc.secret))
			returned, err := getExistingConfig(client, tc.kb)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.expectKey != "" {
				require.NotNil(t, returned)
				assert.True(t, (*ucfg.Config)(returned).HasField("xpack"))
			}
		})
	}
}
