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
  name: "testkb"
  ssl:
    enabled: true
    key: /mnt/elastic-internal/http-certs/tls.key
    certificate: /mnt/elastic-internal/http-certs/tls.crt
xpack:
  security:
    encryptionKey: thisismyencryptionkey
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
	defaultKb := mkKibana()
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(defaultKb),
			Namespace: defaultKb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey"),
		},
	}
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
				client: k8s.WrappedFakeClient(existingSecret),
				kb:     mkKibana,
			},
			want: defaultConfig,
		},
		{
			name: "without TLS",
			args: args{
				client: k8s.WrappedFakeClient(existingSecret),
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
				require.NoError(t, err, "cfg", cfg)
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
					existingSecret,
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "auth-secret",
							Namespace: mkKibana().Namespace,
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
				client: k8s.WrappedFakeClient(existingSecret),
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
		{
			name: "test existing secret does not prevent updates to config, e.g. spec takes precedence even if there is a secret indicating otherwise",
			args: args{
				client: k8s.WrappedFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName(defaultKb),
						Namespace: defaultKb.Namespace,
					},
					Data: map[string][]byte{
						SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey\nlogging.verbose: true"),
					},
				}),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec = kbv1.KibanaSpec{
						Config: &commonv1.Config{
							Data: map[string]interface{}{
								"logging.verbose": false,
							},
						},
					}
					return kb
				},
			},
			want: append(defaultConfig, []byte(`logging.verbose: false`)...),
		},
		{
			name: "test existing secret does not prevent removing items from config in spec",
			args: args{
				client: k8s.WrappedFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName(defaultKb),
						Namespace: defaultKb.Namespace,
					},
					Data: map[string][]byte{
						SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey\nlogging.verbose: true"),
					},
				}),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					return kb
				},
			},
			want: append(defaultConfig),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versionSpecificCfg := settings.MustCanonicalConfig(map[string]interface{}{"elasticsearch.hosts": nil})
			got, err := NewConfigSettings(tt.args.client, tt.args.kb(), versionSpecificCfg)
			if tt.wantErr {
				require.Error(t, err)
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

// TestNewConfigSettingsCreateEncryptionKey checks that we generate a new key if none is specified
func TestNewConfigSettingsCreateEncryptionKey(t *testing.T) {
	client := k8s.WrapClient(fake.NewFakeClient())
	kb := mkKibana()
	got, err := NewConfigSettings(client, kb, nil)
	require.NoError(t, err)
	val, err := (*ucfg.Config)(got.CanonicalConfig).String(XpackSecurityEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.NotEmpty(t, val)
}

// TestNewConfigSettingsExistingEncryptionKey tests that we do not override the existing key if one is already specified
func TestNewConfigSettingsExistingEncryptionKey(t *testing.T) {
	kb := mkKibana()
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(kb),
			Namespace: kb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey"),
		},
	}
	client := k8s.WrapClient(fake.NewFakeClient(existingSecret))
	got, err := NewConfigSettings(client, kb, nil)
	require.NoError(t, err)
	var gotCfg map[string]interface{}
	require.NoError(t, got.Unpack(&gotCfg))
	val, err := (*ucfg.Config)(got.CanonicalConfig).String(XpackSecurityEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.Equal(t, "thisismyencryptionkey", val)
}

// TestNewConfigSettingsExplicitEncryptionKey tests that we do not override the existing key if one is already specified in the Spec
// this should not be done since it is a secure setting, but just in case it happens we do not want to ignore it
func TestNewConfigSettingsExplicitEncryptionKey(t *testing.T) {
	kb := mkKibana()
	key := "thisismyencryptionkey"
	cfg := commonv1.NewConfig(map[string]interface{}{
		XpackSecurityEncryptionKey: key,
	})
	kb.Spec.Config = &cfg
	client := k8s.WrapClient(fake.NewFakeClient())
	got, err := NewConfigSettings(client, kb, nil)
	require.NoError(t, err)
	var gotCfg map[string]interface{}
	require.NoError(t, got.Unpack(&gotCfg))
	val, err := (*ucfg.Config)(got.CanonicalConfig).String(XpackSecurityEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.Equal(t, key, val)
}

func mkKibana() kbv1.Kibana {
	kb := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testkb",
			Namespace: "testns",
		},
	}
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
		name   string
		kb     kbv1.Kibana
		secret corev1.Secret
		// an empty string means we should expect a nil return, anything else should be a key in the parsed config
		expectKey string
	}{
		{
			name:      "happy path",
			kb:        testKb,
			secret:    testValidSecret,
			expectKey: "xpack",
		},
		{
			name:      "no secret exists",
			kb:        testKb,
			secret:    corev1.Secret{},
			expectKey: "",
		},
		{
			name:      "no kibana.yml exists in secret",
			kb:        testKb,
			secret:    testNoYaml,
			expectKey: "",
		},
		{
			name:      "cannot parse yaml",
			kb:        testKb,
			secret:    testInvalidYaml,
			expectKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(&tc.secret))
			result := getExistingConfig(client, tc.kb)
			if tc.expectKey == "" {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.True(t, (*ucfg.Config)(result).HasField(tc.expectKey))
			}
		})
	}
}

func Test_filterExistingConfig(t *testing.T) {
	cfg, err := settings.NewCanonicalConfigFrom(map[string]interface{}{
		XpackSecurityEncryptionKey: "value",
		"notakey":                  "notavalue",
	})
	require.NoError(t, err)
	want, err := settings.NewCanonicalConfigFrom(map[string]interface{}{
		XpackSecurityEncryptionKey: "value",
	})
	require.NoError(t, err)
	filtered := filterExistingConfig(cfg)
	assert.Equal(t, want, filtered)
}
