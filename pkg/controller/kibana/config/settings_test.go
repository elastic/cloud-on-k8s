// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"context"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ucfg "github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var defaultConfig = []byte(`
elasticsearch:
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

func Test_reuseOrGenerateSecrets(t *testing.T) {
	defaultKb := mkKibana()
	type args struct {
		c      k8s.Client
		kibana kbv1.Kibana
	}
	tests := []struct {
		name      string
		args      args
		assertion func(*testing.T, *settings.CanonicalConfig, error)
		wantErr   bool
	}{
		{
			name: "Do not override existing encryption keys",
			args: args{
				c: k8s.WrappedFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: SecretName(defaultKb)},
						Data: map[string][]byte{
							SettingsFilename: defaultConfig,
						},
					},
				),
				kibana: defaultKb,
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				expectedSettings := settings.MustCanonicalConfig(map[string]interface{}{
					"xpack.security.encryptionKey": "thisismyencryptionkey",
				})
				assert.Equal(t, expectedSettings, got)
			},
		},
		{
			name: "Create new encryption keys",
			args: args{
				c: k8s.WrappedFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: SecretName(defaultKb)},
						Data: map[string][]byte{
							SettingsFilename: associationConfig,
						},
					},
				),
				kibana: defaultKb,
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default reusable settings have been generated
				var r reusableSettings
				assert.NoError(t, got.Unpack(&r))
				assert.Equal(t, len(r.EncryptionKey), 64) // Kibana encryption key length should be 64
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOrCreateReusableSettings(tt.args.c, tt.args.kibana)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOrCreateReusableSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.assertion(t, got, err)
		})
	}
}

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
			want: defaultConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := tt.args.kb()
			v := version.From(7, 5, 0)
			got, err := NewConfigSettings(context.Background(), tt.args.client, kb, v)
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

			require.Equal(t, wantCfg, gotCfg)
		})
	}
}

// TestNewConfigSettingsCreateEncryptionKey checks that we generate a new key if none is specified
func TestNewConfigSettingsCreateEncryptionKey(t *testing.T) {
	client := k8s.WrapClient(fake.NewFakeClient())
	kb := mkKibana()
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v)
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
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v)
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
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v)
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
		Spec: kbv1.KibanaSpec{Version: "7.5.0"},
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
		expectErr bool
	}{
		{
			name:      "happy path",
			kb:        testKb,
			secret:    testValidSecret,
			expectKey: "xpack",
			expectErr: false,
		},
		{
			name:      "no secret exists",
			kb:        testKb,
			secret:    corev1.Secret{},
			expectKey: "",
			expectErr: false,
		},
		{
			name:      "no kibana.yml exists in secret",
			kb:        testKb,
			secret:    testNoYaml,
			expectKey: "",
			expectErr: true,
		},
		{
			name:      "cannot parse yaml",
			kb:        testKb,
			secret:    testInvalidYaml,
			expectKey: "",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(&tc.secret))
			result, err := getExistingConfig(client, tc.kb)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tc.expectKey != "" {

				require.NotNil(t, result)
				assert.True(t, (*ucfg.Config)(result).HasField(tc.expectKey))
			}
		})
	}
}
