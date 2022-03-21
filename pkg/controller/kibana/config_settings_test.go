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

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var defaultConfig = []byte(`
elasticsearch:
server:
  host: "0.0.0.0"
  name: "testkb"
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
  name: "testkb"
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
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: SecretName(defaultKb)},
						Data: map[string][]byte{
							SettingsFilename: defaultConfig,
						},
					},
				),
				kibana: defaultKb,
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				t.Helper()
				expectedSettings := settings.MustCanonicalConfig(map[string]interface{}{
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
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: SecretName(defaultKb)},
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
				assert.Equal(t, len(r.EncryptionKey), 64) // key length should be 64
				assert.Equal(t, len(r.ReportingKey), 64)
				assert.Equal(t, len(r.SavedObjectsKey), 64)
			},
		},

		{
			name: "Create new encryption keys pre-7.6.0",
			args: args{
				c: k8s.NewFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: defaultKb.Namespace, Name: SecretName(defaultKb)},
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
				assert.Equal(t, len(r.EncryptionKey), 64) // key length should be 64
				assert.Equal(t, len(r.ReportingKey), 64)
				assert.Equal(t, len(r.SavedObjectsKey), 0) // is only introduced in 7.6.0
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
			SettingsFilename: []byte("xpack.security.encryptionKey: thisismyencryptionkey\nxpack.reporting.encryptionKey: thisismyreportingkey\nxpack.encryptedSavedObjects.encryptionKey: thisismyobjectkey"),
		},
	}
	type args struct {
		client   k8s.Client
		kb       func() kbv1.Kibana
		ipFamily corev1.IPFamily
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
						Data: map[string]interface{}{
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
			name: "test existing secret does not prevent updates to config, e.g. spec takes precedence even if there is a secret indicating otherwise",
			args: args{
				client: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName(defaultKb),
						Namespace: defaultKb.Namespace,
					},
					Data: map[string][]byte{
						SettingsFilename: append(defaultConfig, []byte(`logging.verbose: true`)...),
					},
				}),
				kb: func() kbv1.Kibana {
					kb := mkKibana()
					kb.Spec.Config = &commonv1.Config{
						Data: map[string]interface{}{
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
						Name:      SecretName(defaultKb),
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
			got, err := NewConfigSettings(context.Background(), tt.args.client, kb, v, tt.args.ipFamily)
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

			assert.Empty(t, deep.Equal(wantCfg, gotCfg))
		})
	}
}

// TestNewConfigSettingsCreateEncryptionKeys checks that we generate new keys if none are specified
func TestNewConfigSettingsCreateEncryptionKeys(t *testing.T) {
	client := k8s.NewFakeClient()
	kb := mkKibana()
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol)
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
			Name:      SecretName(kb),
			Namespace: kb.Namespace,
		},
		Data: map[string][]byte{
			SettingsFilename: []byte(fmt.Sprintf("%s: %s\n%s: %s\n%s: %s", XpackSecurityEncryptionKey, securityKey, XpackReportingEncryptionKey, reportKey, XpackEncryptedSavedObjectsEncryptionKey, savedObjsKey)),
		},
	}
	client := k8s.NewFakeClient(existingSecret)
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol)
	require.NoError(t, err)
	var gotCfg map[string]interface{}
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
	cfg := commonv1.NewConfig(map[string]interface{}{
		XpackSecurityEncryptionKey: key,
	})
	kb.Spec.Config = &cfg
	client := k8s.NewFakeClient()
	v := version.MustParse(kb.Spec.Version)
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol)
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
	got, err := NewConfigSettings(context.Background(), client, kb, v, corev1.IPv4Protocol)
	require.NoError(t, err)
	assert.Equal(t, 0, len(got.CanonicalConfig.HasKeys([]string{XpackEncryptedSavedObjects})))
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
			client := k8s.NewFakeClient(&tc.secret)
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
