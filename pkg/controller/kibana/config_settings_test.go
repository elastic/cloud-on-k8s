// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"
	"testing"

	ucfg "github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var defaultConfig = []byte(`
elasticsearch:
server:
  host: "0.0.0.0"
  ssl:
    enabled: true
    key: /mnt/elastic-internal/http-certs/tls.key
    certificate: /mnt/elastic-internal/http-certs/tls.crt
xpack:
  encryptedSavedObjects:
    encryptionKey: thisismyobjectkey
  license_management.ui.enabled: false
  reporting:
    encryptionKey: thisismyreportingkey
  security:
    encryptionKey: thisismyencryptionkey
  monitoring.ui.container.elasticsearch.enabled: true
`)

var defaultConfig8 = []byte(`
elasticsearch:
server:
  host: "0.0.0.0"
  ssl:
    enabled: true
    key: /mnt/elastic-internal/http-certs/tls.key
    certificate: /mnt/elastic-internal/http-certs/tls.crt
xpack:
  encryptedSavedObjects:
    encryptionKey: thisismyobjectkey
  license_management.ui.enabled: false
  reporting:
    encryptionKey: thisismyreportingkey
  security:
    encryptionKey: thisismyencryptionkey
monitoring.ui.container.elasticsearch.enabled: true
`)

var esAssociationConfig = []byte(`
elasticsearch:
  hosts:
    - "https://es-url:9200"
  username: "elastic"
  password: "password"
  ssl:
    certificateAuthorities: /usr/share/kibana/config/elasticsearch-certs/ca.crt
    verificationMode: certificate
`)

var entAssociationConfig = []byte(`
enterpriseSearch:
  host: https://ent-url:3002
  ssl:
    certificateAuthorities: /usr/share/kibana/config/ent-certs/ca.crt
    verificationMode: certificate
`)

func Test_reuseOrGenerateSecrets(t *testing.T) {
	defaultKb := mkKibana()
	type args struct {
		c      k8s.Client
		kibana kbv1.Kibana
	}

	kb75 := mkKibana()
	kb75.Spec.Version = "7.5.0"

	tests := []struct {
		name      string
		args      args
		assertion func(*testing.T, *settings.CanonicalConfig, error)
		wantErr   bool
	}{
		{
			name: "Do not override existing encryption keys",
			args: args{
				c: k8s.NewFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: kbv1.ConfigSecret(defaultKb.Name)},
						Data: map[string][]byte{
							SettingsFilename: defaultConfig,
						},
					},
				),
				kibana: defaultKb,
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				t.Helper()
				expectedSettings := settings.MustCanonicalConfig(map[string]any{
					XpackSecurityEncryptionKey:              "thisismyencryptionkey",
					XpackReportingEncryptionKey:             "thisismyreportingkey",
					XpackEncryptedSavedObjectsEncryptionKey: "thisismyobjectkey",
				})
				assert.Equal(t, expectedSettings, got)
			},
		},
		{
			name: "Create new encryption keys",
			args: args{
				c: k8s.NewFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: kbv1.ConfigSecret(defaultKb.Name)},
						Data: map[string][]byte{
							SettingsFilename: esAssociationConfig,
						},
					},
				),
				kibana: defaultKb,
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				t.Helper()
				// Unpack the configuration to check that some default reusable settings have been generated
				var r reusableSettings
				assert.NoError(t, got.Unpack(&r))
				assert.Equal(t, 64, len(r.EncryptionKey)) // key length should be 64
				assert.Equal(t, 64, len(r.ReportingKey))
				assert.Equal(t, 64, len(r.SavedObjectsKey))
			},
		},

		{
			name: "Create new encryption keys pre-7.6.0",
			args: args{
				c: k8s.NewFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: kbv1.ConfigSecret(defaultKb.Name)},
						Data: map[string][]byte{
							SettingsFilename: esAssociationConfig,
						},
					},
				),
				kibana: kb75,
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				t.Helper()
				// Unpack the configuration to check that some default reusable settings have been generated
				var r reusableSettings
				assert.NoError(t, got.Unpack(&r))
				assert.Equal(t, 64, len(r.EncryptionKey)) // key length should be 64
				assert.Equal(t, 64, len(r.ReportingKey))
				assert.Equal(t, 0, len(r.SavedObjectsKey)) // is only introduced in 7.6.0
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOrCreateReusableSettings(context.Background(), tt.args.c, tt.args.kibana)
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
			Name:      kbv1.ConfigSecret(defaultKb.Name),
			Namespace: defaultKb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey\nxpack.reporting.encryptionKey: thisismyreportingkey\nxpack.encryptedSavedObjects.encryptionKey: thisismyobjectkey"),
		},
	}
	type args struct {
		client                 k8s.Client
		kb                     func() kbv1.Kibana
		ipFamily               corev1.IPFamily
		kibanaConfigFromPolicy *settings.CanonicalConfig
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "default config IPv4",
			args: args{
				client:   k8s.NewFakeClient(existingSecret),
				kb:       mkKibana,
				ipFamily: corev1.IPv4Protocol,
			},
			want: defaultConfig,
		},
		{
			name: "default config IPv6",
			args: args{
				client:   k8s.NewFakeClient(existingSecret),
				kb:       mkKibana,
				ipFamily: corev1.IPv6Protocol,
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig)
				require.NoError(t, err, "cfg", cfg)
				err = (*ucfg.Config)(cfg).SetString("server.host", -1, "::", settings.Options...)
				require.NoError(t, err)
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
		},
		{
			name: "without TLS",
			args: args{
				client: k8s.NewFakeClient(existingSecret),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.HTTP = commonv1.HTTPConfig{
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								Disabled: true,
							},
						},
					}
					return kb
				},
				ipFamily: corev1.IPv4Protocol,
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
			name: "with elasticsearch Association",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.Version = "8.0.0" // to use service accounts
					kb.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "test-es"}
					kb.EsAssociation().SetAssociationConf(&commonv1.AssociationConf{
						AuthSecretName:   "auth-secret",
						AuthSecretKey:    "token",
						CASecretName:     "ca-secret",
						CACertProvided:   true,
						IsServiceAccount: true,
						URL:              "https://es-url:9200",
					})
					return kb
				},
				client: k8s.NewFakeClient(
					existingSecret,
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "auth-secret",
							Namespace: mkKibana().Namespace,
						},
						Data: map[string][]byte{
							"token": []byte("AAEAAWVsYXN0aWMva2liYW5hL2RlZmF1bHRfa2liYW5hXzRjMWJkZTQzLWFiYjMtNDE0MC1hNDk4LTA4NDRkMDkwZjE3Yjplb3RYYlhDbThtOFgxU2pPelpqdktCcjB3V1NPNHZUQ0FRWU4yWEFNMGRyU1lrYTdNUWJXTHozY1lIVzF3YlZw"),
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
				),
				ipFamily: corev1.IPv4Protocol,
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig8)
				require.NoError(t, err)
				assocCfg, err := settings.ParseConfig(
					[]byte(`elasticsearch:
  hosts:
    - "https://es-url:9200"
  serviceAccountToken: AAEAAWVsYXN0aWMva2liYW5hL2RlZmF1bHRfa2liYW5hXzRjMWJkZTQzLWFiYjMtNDE0MC1hNDk4LTA4NDRkMDkwZjE3Yjplb3RYYlhDbThtOFgxU2pPelpqdktCcjB3V1NPNHZUQ0FRWU4yWEFNMGRyU1lrYTdNUWJXTHozY1lIVzF3YlZw
  ssl:
    certificateAuthorities: /usr/share/kibana/config/elasticsearch-certs/ca.crt
    verificationMode: certificate
`),
				)
				require.NoError(t, err)
				require.NoError(t, cfg.MergeWith(assocCfg))
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
			wantErr: false,
		},
		{
			name: "with elasticsearch Association",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "test-es"}
					kb.EsAssociation().SetAssociationConf(&commonv1.AssociationConf{
						AuthSecretName: "auth-secret",
						AuthSecretKey:  "elastic",
						CASecretName:   "ca-secret",
						CACertProvided: true,
						URL:            "https://es-url:9200",
					})
					return kb
				},
				client: k8s.NewFakeClient(
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
				),
				ipFamily: corev1.IPv4Protocol,
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig)
				require.NoError(t, err)
				assocCfg, err := settings.ParseConfig(esAssociationConfig)
				require.NoError(t, err)
				require.NoError(t, cfg.MergeWith(assocCfg))
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
			wantErr: false,
		},
		{
			name: "with Enterprise Search Association",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.EnterpriseSearchRef = commonv1.ObjectSelector{Name: "test-ent"}
					kb.EntAssociation().SetAssociationConf(&commonv1.AssociationConf{
						AuthSecretName: "-",
						AuthSecretKey:  "",
						CASecretName:   "ent-ca-secret",
						CACertProvided: true,
						URL:            "https://ent-url:3002",
					})
					return kb
				},
				client: k8s.NewFakeClient(
					existingSecret,
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ent-ca-secret",
						},
						Data: map[string][]byte{
							"ca.crt": []byte("certificate"),
						},
					},
				),
				ipFamily: corev1.IPv4Protocol,
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig)
				require.NoError(t, err)
				assocCfg, err := settings.ParseConfig(entAssociationConfig)
				require.NoError(t, err)
				require.NoError(t, cfg.MergeWith(assocCfg))
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
			wantErr: false,
		}, {
			name: "with Elasticsearch and Enterprise Search associations",
			args: args{
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "test-es"}
					kb.EsAssociation().SetAssociationConf(&commonv1.AssociationConf{
						AuthSecretName: "auth-secret",
						AuthSecretKey:  "elastic",
						CASecretName:   "ca-secret",
						CACertProvided: true,
						URL:            "https://es-url:9200",
					})
					kb.Spec.EnterpriseSearchRef = commonv1.ObjectSelector{Name: "test-ent"}
					kb.EntAssociation().SetAssociationConf(&commonv1.AssociationConf{
						AuthSecretName: "-",
						AuthSecretKey:  "",
						CASecretName:   "ent-ca-secret",
						CACertProvided: true,
						URL:            "https://ent-url:3002",
					})
					return kb
				},
				client: k8s.NewFakeClient(
					existingSecret,
					// ent certs
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ent-ca-secret",
						},
						Data: map[string][]byte{
							"ca.crt": []byte("certificate"),
						},
					},
					// es auth
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "auth-secret",
							Namespace: mkKibana().Namespace,
						},
						Data: map[string][]byte{
							"elastic": []byte("password"),
						},
					},
					// es certs
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ca-secret",
						},
						Data: map[string][]byte{
							"ca.crt": []byte("certificate"),
						},
					},
				),
				ipFamily: corev1.IPv4Protocol,
			},
			want: func() []byte {
				cfg, err := settings.ParseConfig(defaultConfig)
				require.NoError(t, err)
				esAssocCfg, err := settings.ParseConfig(esAssociationConfig)
				require.NoError(t, err)
				entAssocCfg, err := settings.ParseConfig(entAssociationConfig)
				require.NoError(t, err)
				require.NoError(t, cfg.MergeWith(esAssocCfg))
				require.NoError(t, cfg.MergeWith(entAssocCfg))
				bytes, err := cfg.Render()
				require.NoError(t, err)
				return bytes
			}(),
			wantErr: false,
		},
		{
			name: "with user config",
			args: args{
				client: k8s.NewFakeClient(existingSecret),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.Config = &commonv1.Config{
						Data: map[string]any{
							"foo": "bar",
						},
					}
					return kb
				},
				ipFamily: corev1.IPv4Protocol,
			},
			want: append(defaultConfig, []byte(`foo: bar`)...),
		},
		{
			name: "with kibana config from stackconfigpolicy",
			args: args{
				client: k8s.NewFakeClient(existingSecret),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.Config = &commonv1.Config{
						Data: map[string]any{
							"foo": "bar",
						},
					}
					return kb
				},
				ipFamily:               corev1.IPv4Protocol,
				kibanaConfigFromPolicy: settings.MustCanonicalConfig(map[string]any{"foo": "bars"}),
			},
			want: append(defaultConfig, []byte(`foo: bars`)...),
		},
		{
			name: "test existing secret does not prevent updates to config, e.g. spec takes precedence even if there is a secret indicating otherwise",
			args: args{
				client: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kbv1.ConfigSecret(defaultKb.Name),
						Namespace: defaultKb.Namespace,
					},
					Data: map[string][]byte{
						SettingsFilename: append(defaultConfig, []byte(`logging.verbose: true`)...),
					},
				}),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.Config = &commonv1.Config{
						Data: map[string]any{
							"logging.verbose": false,
						},
					}
					return kb
				},
				ipFamily: corev1.IPv4Protocol,
			},
			want: append(defaultConfig, []byte(`logging.verbose: false`)...),
		},
		{
			name: "test existing secret does not prevent removing items from config in spec",
			args: args{
				client: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kbv1.ConfigSecret(defaultKb.Name),
						Namespace: defaultKb.Namespace,
					},
					Data: map[string][]byte{
						SettingsFilename: append(defaultConfig, []byte(`logging.verbose: true`)...),
					},
				}),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					return kb
				},
				ipFamily: corev1.IPv4Protocol,
			},
			want: defaultConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := tt.args.kb()
			v := version.From(7, 6, 0)
			got, err := NewConfigSettings(context.Background(), tt.args.client, kb, v, tt.args.ipFamily, tt.args.kibanaConfigFromPolicy)
			if tt.wantErr {
				require.Error(t, err)
			}
			require.NoError(t, err)

			// convert "got" into something comparable
			var gotCfg map[string]any
			require.NoError(t, got.Unpack(&gotCfg))

			// convert "want" into something comparable
			cfg, err := uyaml.NewConfig(tt.want, commonv1.CfgOptions...)
			require.NoError(t, err)
			var wantCfg map[string]any
			require.NoError(t, cfg.Unpack(&wantCfg))

			assert.Empty(t, deep.Equal(wantCfg, gotCfg))
		})
	}
}

// TestNewConfigSettingsCreateEncryptionKeys checks that we generate new keys if none are specified
func TestNewConfigSettingsCreateEncryptionKeys(t *testing.T) {
	client := k8s.NewFakeClient()
	kb := mkKibana()
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol, nil)
	require.NoError(t, err)
	for _, key := range []string{XpackSecurityEncryptionKey, XpackReportingEncryptionKey, XpackEncryptedSavedObjectsEncryptionKey} {
		val, err := (*ucfg.Config)(got.CanonicalConfig).String(key, -1, settings.Options...)
		require.NoError(t, err)
		assert.NotEmpty(t, val)
	}
}

// TestNewConfigSettingsExistingEncryptionKey tests that we do not override the existing key if one is already specified
func TestNewConfigSettingsExistingEncryptionKey(t *testing.T) {
	kb := mkKibana()
	securityKey := "securityKey"
	reportKey := "reportKey"
	savedObjsKey := "savedObjsKey"
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbv1.ConfigSecret(kb.Name),
			Namespace: kb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: fmt.Appendf(nil, "%s: %s\n%s: %s\n%s: %s", XpackSecurityEncryptionKey, securityKey, XpackReportingEncryptionKey, reportKey, XpackEncryptedSavedObjectsEncryptionKey, savedObjsKey),
		},
	}
	client := k8s.NewFakeClient(existingSecret)
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol, nil)
	require.NoError(t, err)
	var gotCfg map[string]any
	require.NoError(t, got.Unpack(&gotCfg))

	val, err := (*ucfg.Config)(got.CanonicalConfig).String(XpackSecurityEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.Equal(t, securityKey, val)

	val, err = (*ucfg.Config)(got.CanonicalConfig).String(XpackReportingEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.Equal(t, reportKey, val)

	val, err = (*ucfg.Config)(got.CanonicalConfig).String(XpackEncryptedSavedObjectsEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.Equal(t, savedObjsKey, val)
}

// TestNewConfigSettingsExplicitEncryptionKey tests that we do not override the existing key if one is already specified in the Spec
// this should not be done since it is a secure setting, but just in case it happens we do not want to ignore it
func TestNewConfigSettingsExplicitEncryptionKey(t *testing.T) {
	kb := mkKibana()
	key := "thisismyencryptionkey"
	cfg := commonv1.NewConfig(map[string]any{
		XpackSecurityEncryptionKey: key,
	})
	kb.Spec.Config = &cfg
	client := k8s.NewFakeClient()
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol, nil)
	require.NoError(t, err)
	val, err := (*ucfg.Config)(got.CanonicalConfig).String(XpackSecurityEncryptionKey, -1, settings.Options...)
	require.NoError(t, err)
	assert.Equal(t, key, val)
}

// Verifies that pre-7.6.0 keys are not present in the config
func TestNewConfigSettingsPre760(t *testing.T) {
	kb := mkKibana()
	kb.Spec.Version = "7.5.0"
	client := k8s.NewFakeClient()
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(got.CanonicalConfig.HasKeys([]string{XpackEncryptedSavedObjects})))
}

func TestNewConfigSettingsFleetOutputsInjection(t *testing.T) {
	baseKibana := func(configData map[string]any) kbv1.Kibana {
		kb := mkKibana()
		kb.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "test-es"}
		kb.EsAssociation().SetAssociationConf(&commonv1.AssociationConf{
			AuthSecretName: "auth-secret",
			AuthSecretKey:  "elastic",
			CASecretName:   "ca-secret",
			CACertProvided: true,
			URL:            "https://es-url:9200",
		})
		if configData != nil {
			kb.Spec.Config = &commonv1.Config{Data: configData}
		}
		return kb
	}

	clientForKibana := func(kb kbv1.Kibana) k8s.Client {
		return k8s.NewFakeClient(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kbv1.ConfigSecret(kb.Name),
					Namespace: kb.Namespace,
				},
				Data: map[string][]byte{
					SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey\nxpack.reporting.encryptionKey: thisismyreportingkey\nxpack.encryptedSavedObjects.encryptionKey: thisismyobjectkey"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auth-secret",
					Namespace: kb.Namespace,
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
		)
	}

	testCases := []struct {
		name                string
		configData          map[string]any
		wantInjectedDefault bool
		wantOutputHosts     []string
		wantHostsPresent    bool
	}{
		{
			name: "injects outputs when packages configured and outputs missing",
			configData: map[string]any{
				XpackFleetPackages:                 []any{map[string]any{"name": "fleet_server", "version": "latest"}},
				XpackFleetAgentsElasticsearchHosts: []any{"https://elasticsearch-es-http.default.svc:9200"},
			},
			wantInjectedDefault: true,
			wantOutputHosts:     []string{"https://es-url:9200"},
			wantHostsPresent:    false,
		},
		{
			name: "injects outputs when packages configured and outputs empty",
			configData: map[string]any{
				XpackFleetPackages:                 []any{map[string]any{"name": "fleet_server", "version": "latest"}},
				XpackFleetOutputs:                  []any{},
				XpackFleetAgentsElasticsearchHosts: []any{"https://elasticsearch-es-http.default.svc:9200"},
			},
			wantInjectedDefault: true,
			wantOutputHosts:     []string{"https://es-url:9200"},
			wantHostsPresent:    false,
		},
		{
			name: "does not override existing outputs when packages configured",
			configData: map[string]any{
				XpackFleetPackages:                 []any{map[string]any{"name": "fleet_server", "version": "latest"}},
				XpackFleetAgentsElasticsearchHosts: []any{"https://elasticsearch-es-http.default.svc:9200"},
				XpackFleetOutputs: []any{
					map[string]any{
						"id":         "custom-output",
						"is_default": true,
						"name":       "custom",
						"type":       "elasticsearch",
						"hosts":      []any{"https://custom-es:9200"},
					},
				},
			},
			wantInjectedDefault: false,
			wantOutputHosts:     []string{"https://custom-es:9200"},
			wantHostsPresent:    false,
		},
		{
			name: "does not inject outputs when packages are absent",
			configData: map[string]any{
				XpackFleetAgentsElasticsearchHosts:      []any{"https://elasticsearch-es-http.default.svc:9200"},
				"xpack.fleet.agents.fleet_server.hosts": []any{"https://fleet-server-agent-http.default.svc:8220"},
			},
			wantInjectedDefault: false,
			wantOutputHosts:     nil,
			wantHostsPresent:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kb := baseKibana(tc.configData)
			v := version.MustParse(kb.Spec.Version)
			got, err := NewConfigSettings(context.Background(), clientForKibana(kb), kb, v, corev1.IPv4Protocol, nil)
			require.NoError(t, err)

			var fleetCfg struct {
				Outputs []any `config:"xpack.fleet.outputs"`
			}
			require.NoError(t, got.CanonicalConfig.Unpack(&fleetCfg))
			outputs := fleetCfg.Outputs
			require.NoError(t, err)
			found := len(got.CanonicalConfig.HasKeys([]string{XpackFleetOutputs})) > 0
			hostsFound := len(got.CanonicalConfig.HasKeys([]string{XpackFleetAgentsElasticsearchHosts})) > 0
			require.Equal(t, tc.wantHostsPresent, hostsFound)
			if tc.wantOutputHosts == nil {
				require.False(t, found)
				return
			}
			require.True(t, found)
			require.NotEmpty(t, outputs)

			firstOutput, ok := outputs[0].(map[string]any)
			require.True(t, ok)
			hosts, ok := firstOutput["hosts"].([]any)
			require.True(t, ok)
			require.Len(t, hosts, len(tc.wantOutputHosts))
			for i, expectedHost := range tc.wantOutputHosts {
				require.Equal(t, expectedHost, hosts[i])
			}
			if tc.wantInjectedDefault {
				require.Equal(t, ECKFleetOutputID, firstOutput["id"])
			}
		})
	}
}

func Test_defaultFleetOutputsConfig(t *testing.T) {
	withCA := commonv1.AssociationConf{
		URL:            "https://es-url:9200",
		CACertProvided: true,
		CASecretName:   "ca-secret",
	}
	withoutCA := commonv1.AssociationConf{
		URL:            "https://es-url:9200",
		CACertProvided: false,
	}

	tests := []struct {
		name      string
		assocConf commonv1.AssociationConf
		kb        kbv1.Kibana
		wantSSL   bool
		wantCA    []any
	}{
		{
			name:      "with CA",
			assocConf: withCA,
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: "elasticsearch"}},
			},
			wantSSL: true,
			wantCA:  []any{"/mnt/elastic-internal/elasticsearch-association/default/elasticsearch/certs/ca.crt"},
		},
		{
			name:      "without CA",
			assocConf: withoutCA,
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: "elasticsearch"}},
			},
			wantSSL: false,
			wantCA:  nil,
		},
		{
			name:      "with CA and explicit es namespace",
			assocConf: withCA,
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Namespace: "kibana-ns"},
				Spec: kbv1.KibanaSpec{
					ElasticsearchRef: commonv1.ObjectSelector{
						Name:      "prod-es",
						Namespace: "observability",
					},
				},
			},
			wantSSL: true,
			wantCA:  []any{"/mnt/elastic-internal/elasticsearch-association/observability/prod-es/certs/ca.crt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultFleetOutputsConfig(tt.assocConf, tt.kb.EsAssociation())
			var fleetCfg struct {
				Outputs []any `config:"xpack.fleet.outputs"`
			}
			require.NoError(t, cfg.Unpack(&fleetCfg))
			outputs := fleetCfg.Outputs
			require.Len(t, outputs, 1)
			entry, ok := outputs[0].(map[string]any)
			require.True(t, ok)
			require.Equal(t, ECKFleetOutputID, entry["id"])
			require.Equal(t, ECKFleetOutputName, entry["name"])
			require.Equal(t, "elasticsearch", entry["type"])
			require.Equal(t, tt.wantSSL, entry["ssl"] != nil)
			if tt.wantSSL {
				sslCfg, ok := entry["ssl"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantCA, sslCfg["certificate_authorities"])
			}
		})
	}
}

func Test_maybeConfigureFleetOutputs(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *settings.CanonicalConfig
		esAssoc *commonv1.AssociationConf
		kb      kbv1.Kibana
		wantMap map[string]any
		wantErr bool
	}{
		{
			name: "injects outputs and removes legacy hosts",
			cfg: settings.MustCanonicalConfig(map[string]any{
				XpackFleetPackages:                 []any{map[string]any{"name": "fleet_server"}},
				XpackFleetAgentsElasticsearchHosts: []any{"https://legacy-es:9200"},
			}),
			esAssoc: &commonv1.AssociationConf{
				URL:            "https://es-url:9200",
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "elastic",
				CACertProvided: true,
				CASecretName:   "ca-secret",
			},
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: "elasticsearch"}},
			},
			wantMap: map[string]any{
				"xpack": map[string]any{
					"fleet": map[string]any{
						"agents": map[string]any{
							"elasticsearch": nil,
						},
						"packages": []any{map[string]any{"name": "fleet_server"}},
						"outputs": []any{
							map[string]any{
								"id":         ECKFleetOutputID,
								"is_default": true,
								"name":       ECKFleetOutputName,
								"type":       "elasticsearch",
								"hosts":      []any{"https://es-url:9200"},
								"ssl": map[string]any{
									"certificate_authorities": []any{"/mnt/elastic-internal/elasticsearch-association/default/elasticsearch/certs/ca.crt"},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "keeps legacy hosts when outputs are absent",
			cfg: settings.MustCanonicalConfig(map[string]any{
				XpackFleetAgentsElasticsearchHosts: []any{"https://legacy-es:9200"},
			}),
			esAssoc: nil,
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: "elasticsearch"}},
			},
			wantMap: map[string]any{
				"xpack": map[string]any{
					"fleet": map[string]any{
						"agents": map[string]any{
							"elasticsearch": map[string]any{
								"hosts": []any{"https://legacy-es:9200"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := maybeConfigureFleetOutputs(tt.cfg, tt.esAssoc, tt.kb.EsAssociation())
			require.Equal(t, tt.wantErr, err != nil)
			if tt.wantErr {
				return
			}

			var got map[string]any
			require.NoError(t, tt.cfg.Unpack(&got))
			require.Empty(t, deep.Equal(tt.wantMap, got))
		})
	}
}

func mkKibana() kbv1.Kibana {
	kb := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testkb",
			Namespace: "testns",
		},
		Spec: kbv1.KibanaSpec{Version: "7.6.0"},
	}
	return kb
}

func Test_getExistingConfig(t *testing.T) {
	testKb := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testkb",
			Namespace: "testns",
		},
		Spec: kbv1.KibanaSpec{
			Version: "7.6.0",
		},
	}
	testValidSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbv1.ConfigSecret(testKb.Name),
			Namespace: testKb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: defaultConfig,
		},
	}
	testNoYaml := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbv1.ConfigSecret(testKb.Name),
			Namespace: testKb.Namespace,
		},
		Data: map[string][]byte{
			"notarealkey": []byte(`:-{`),
		},
	}
	testInvalidYaml := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbv1.ConfigSecret(testKb.Name),
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
			client := k8s.NewFakeClient(&tc.secret)
			result, err := getExistingConfig(context.Background(), client, tc.kb)
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
