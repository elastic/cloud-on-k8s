// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func entWithConfigRef(secretName string) entv1beta1.EnterpriseSearch {
	ent := entv1beta1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "ent",
		},
	}
	if secretName != "" {
		ent.Spec.ConfigRef = &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: secretName}}
	}
	return ent
}

func secretWithConfig(name string, cfg []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
		},
		Data: map[string][]byte{
			ConfigFilename: cfg,
		},
	}
}

func entWithAssociation(name string, associationConf commonv1.AssociationConf) entv1beta1.EnterpriseSearch {
	ent := entv1beta1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
		},
	}
	ent.SetAssociationConf(&associationConf)
	return ent
}

func Test_parseConfigRef(t *testing.T) {
	tests := []struct {
		name        string
		secrets     []runtime.Object
		ent         entv1beta1.EnterpriseSearch
		wantConfig  *settings.CanonicalConfig
		wantWatches bool
		wantErr     bool
	}{
		{
			name:        "no configRef specified",
			secrets:     nil,
			ent:         entWithConfigRef(""),
			wantConfig:  nil,
			wantWatches: false,
		},
		{
			name:        "a referenced secret does not exist: return an error",
			secrets:     []runtime.Object{},
			ent:         entWithConfigRef("non-existing"),
			wantConfig:  nil,
			wantWatches: true,
			wantErr:     true,
		},
		{
			name: "a referenced secret is invalid: return an error",
			secrets: []runtime.Object{
				secretWithConfig("invalid", []byte("invalidyaml")),
			},
			ent:         entWithConfigRef("invalid"),
			wantConfig:  nil,
			wantWatches: true,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.secrets...)
			w := watches.NewDynamicWatches()
			driver := &ReconcileEnterpriseSearch{dynamicWatches: w, Client: c, recorder: record.NewFakeRecorder(10)}
			got, err := parseConfigRef(driver, tt.ent)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantConfig, got)
			if tt.wantWatches {
				require.Len(t, w.Secrets.Registrations(), 1)
			} else {
				require.Len(t, w.Secrets.Registrations(), 0)
			}
		})
	}
}

func Test_reuseOrGenerateSecrets(t *testing.T) {
	type args struct {
		c   k8s.Client
		ent entv1beta1.EnterpriseSearch
	}
	tests := []struct {
		name      string
		args      args
		assertion func(*testing.T, *settings.CanonicalConfig, error)
		wantErr   bool
	}{
		{
			name: "Do not override existing keys",
			args: args{
				c: k8s.WrappedFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample-ent-config"},
						Data: map[string][]byte{
							ConfigFilename: []byte(existingConfigWithReusableSettings),
						},
					},
				),
				ent: entv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				expectedSettings := settings.MustCanonicalConfig(map[string]interface{}{
					SecretSessionSetting:  "alreadysetsessionkey",
					EncryptionKeysSetting: []string{"alreadysetencryptionkey1", "alreadysetencryptionkey2"},
				})
				assert.Equal(t, expectedSettings, got)
			},
		},
		{
			name: "Do not override existing encryption keys, create missing session key",
			args: args{
				c: k8s.WrappedFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample-ent-config"},
						Data: map[string][]byte{
							ConfigFilename: []byte(existingConfig),
						},
					},
				),
				ent: entv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default reusable settings have been generated
				var e reusableSettings
				assert.NoError(t, got.Unpack(&e))
				assert.Equal(t, len(e.EncryptionKeys), 1)     // We set 1 encryption key by default
				assert.Equal(t, len(e.EncryptionKeys[0]), 32) // encryption key length should be 32
				assert.Equal(t, len(e.SecretSession), 32)     // session key length should be 24
			},
		},
		{
			name: "Create missing keys",
			args: args{
				c: k8s.WrappedFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample-ent-config"},
						Data: map[string][]byte{
							ConfigFilename: []byte(existingConfig),
						},
					},
				),
				ent: entv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default reusable settings have been generated
				var e reusableSettings
				assert.NoError(t, got.Unpack(&e))
				assert.Equal(t, len(e.EncryptionKeys), 1)     // We set 1 encryption key by default
				assert.Equal(t, len(e.EncryptionKeys[0]), 32) // encryption key length should be 32
				assert.Equal(t, len(e.SecretSession), 32)     // session key length should be 32
			},
		},
		{
			name: "No configuration to reuse",
			args: args{
				c: k8s.WrappedFakeClient(),
				ent: entv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default reusable settings have been generated
				var e reusableSettings
				assert.NoError(t, got.Unpack(&e))
				assert.Equal(t, len(e.EncryptionKeys), 1)     // We set 1 encryption key by default
				assert.Equal(t, len(e.EncryptionKeys[0]), 32) // encryption key length should be 32
				assert.Equal(t, len(e.SecretSession), 32)     // session key length should be 32
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOrCreateReusableSettings(tt.args.c, tt.args.ent)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOrCreateReusableSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.assertion(t, got, err)
		})
	}
}

func TestReconcileConfig(t *testing.T) {
	tests := []struct {
		name        string
		runtimeObjs []runtime.Object
		ent         entv1beta1.EnterpriseSearch
		// we don't compare the exact secret content because some keys are random, but we at least check
		// all entries in this array exist in the reconciled secret, and there are not more rows
		wantSecretEntries []string
	}{
		{
			name:        "create default config secret",
			runtimeObjs: nil,
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
			},
			wantSecretEntries: []string{
				"allow_es_settings_modification: true",
				"ent_search:",
				"external_url: https://localhost:3002",
				"filebeat_log_directory: /var/log/enterprise-search",
				"listen_host: 0.0.0.0",
				"log_directory: /var/log/enterprise-search",
				"ssl:",
				"certificate: /mnt/elastic-internal/http-certs/tls.crt",
				"certificate_authorities:",
				"- /mnt/elastic-internal/http-certs/ca.crt",
				"enabled: true",
				"key: /mnt/elastic-internal/http-certs/tls.key",
				"secret_management:",
				"encryption_keys:",
				"-",                   // don't check the actual encryption key
				"secret_session_key:", // don't check the actual secret session key
			},
		},
		{
			name: "update existing default config secret",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sample-ent-config",
						// missing labels
					},
					Data: map[string][]byte{
						// missing config settings
						"enterprise-search.yml": []byte("allow_es_settings_modification: true"),
					},
				},
			},
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
			},
			wantSecretEntries: []string{
				"allow_es_settings_modification: true",
				"ent_search:",
				"external_url: https://localhost:3002",
				"filebeat_log_directory: /var/log/enterprise-search",
				"listen_host: 0.0.0.0",
				"log_directory: /var/log/enterprise-search",
				"ssl:",
				"certificate: /mnt/elastic-internal/http-certs/tls.crt",
				"certificate_authorities:",
				"- /mnt/elastic-internal/http-certs/ca.crt",
				"enabled: true",
				"key: /mnt/elastic-internal/http-certs/tls.key",
				"secret_management:",
				"encryption_keys:",
				"-",                   // don't check the actual encryption key
				"secret_session_key:", // don't check the actual secret session key
			},
		},
		{
			name: "with Elasticsearch association",
			ent: entWithAssociation("sample", commonv1.AssociationConf{
				AuthSecretName: "sample-ent-user",
				AuthSecretKey:  "ns-sample-ent-user",
				CACertProvided: true,
				CASecretName:   "sample-ent-es-ca",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			}),
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sample-ent-user",
					},
					Data: map[string][]byte{
						"ns-sample-ent-user": []byte("mypassword"),
					},
				},
			},
			wantSecretEntries: []string{
				"allow_es_settings_modification: true",
				"elasticsearch:",
				"host: https://elasticsearch-sample-es-http.default.svc:9200",
				"password: mypassword",
				"ssl:",
				"certificate_authority: /mnt/elastic-internal/es-certs/tls.crt",
				"enabled: true",
				"username: ns-sample-ent-user",
				"ent_search:",
				"auth:",
				"source: elasticsearch-native",
				"external_url: https://localhost:3002",
				"filebeat_log_directory: /var/log/enterprise-search",
				"listen_host: 0.0.0.0",
				"log_directory: /var/log/enterprise-search",
				"ssl:",
				"certificate: /mnt/elastic-internal/http-certs/tls.crt",
				"certificate_authorities:",
				"- /mnt/elastic-internal/http-certs/ca.crt",
				"enabled: true",
				"key: /mnt/elastic-internal/http-certs/tls.key",
				"secret_management:",
				"encryption_keys:",
				"-",                   // don't check the actual encryption key
				"secret_session_key:", // don't check the actual secret session key
			},
		},
		{
			name:        "with user-provided config overrides",
			runtimeObjs: nil,
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
				Spec: entv1beta1.EnterpriseSearchSpec{
					Config: &commonv1.Config{Data: map[string]interface{}{
						"foo":                     "bar",                    // new setting
						"ent_search.external_url": "https://my.own.dns.com", // override existing setting
					}},
				},
			},
			wantSecretEntries: []string{
				"allow_es_settings_modification: true",
				"ent_search:",
				"external_url: https://my.own.dns.com", // overridden default setting
				"filebeat_log_directory: /var/log/enterprise-search",
				"foo: bar", // new setting
				"listen_host: 0.0.0.0",
				"log_directory: /var/log/enterprise-search",
				"ssl:",
				"certificate: /mnt/elastic-internal/http-certs/tls.crt",
				"certificate_authorities:",
				"- /mnt/elastic-internal/http-certs/ca.crt",
				"enabled: true",
				"key: /mnt/elastic-internal/http-certs/tls.key",
				"secret_management:",
				"encryption_keys:",
				"-",                   // don't check the actual encryption key
				"secret_session_key:", // don't check the actual secret session key
			},
		},
		{
			name: "with user-provided config secret",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "my-config",
					},
					Data: map[string][]byte{
						"enterprise-search.yml": []byte("ent_search.external_url: https://my.own.dns.from.configref.com"),
					},
				},
			},
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
				Spec: entv1beta1.EnterpriseSearchSpec{
					Config: &commonv1.Config{Data: map[string]interface{}{
						"foo":                     "bar",                    // new setting
						"ent_search.external_url": "https://my.own.dns.com", // override existing setting
					}},
					ConfigRef: &commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{SecretName: "my-config"}, // override the external url from config
					},
				},
			},
			wantSecretEntries: []string{
				"allow_es_settings_modification: true",
				"ent_search:",
				"external_url: https://my.own.dns.from.configref.com", // overridden from configRef
				"filebeat_log_directory: /var/log/enterprise-search",
				"foo: bar", // new setting
				"listen_host: 0.0.0.0",
				"log_directory: /var/log/enterprise-search",
				"ssl:",
				"certificate: /mnt/elastic-internal/http-certs/tls.crt",
				"certificate_authorities:",
				"- /mnt/elastic-internal/http-certs/ca.crt",
				"enabled: true",
				"key: /mnt/elastic-internal/http-certs/tls.key",
				"secret_management:",
				"encryption_keys:",
				"-",                   // don't check the actual encryption key
				"secret_session_key:", // don't check the actual secret session key
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &ReconcileEnterpriseSearch{
				Client:         k8s.WrappedFakeClient(tt.runtimeObjs...),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
			}

			// secret metadata should be correct
			got, err := ReconcileConfig(driver, tt.ent)
			require.NoError(t, err)
			assert.Equal(t, "sample-ent-config", got.Name)
			assert.Equal(t, "ns", got.Namespace)
			assert.Equal(t, map[string]string{
				"common.k8s.elastic.co/type":           "enterprise-search",
				"eck.k8s.elastic.co/credentials":       "true",
				"enterprisesearch.k8s.elastic.co/name": "sample",
			}, got.Labels)

			// secret data should contain the expected entries
			data := bytes.TrimRight(got.Data["enterprise-search.yml"], "\n")
			dataEntries := bytes.Split(data, []byte("\n"))
			require.Len(t, dataEntries, len(tt.wantSecretEntries))
			for _, setting := range tt.wantSecretEntries {
				assert.Contains(t, string(got.Data["enterprise-search.yml"]), setting)
			}

			var updatedResource corev1.Secret
			err = driver.K8sClient().Get(k8s.ExtractNamespacedName(&got), &updatedResource)
			assert.NoError(t, err)
			assert.Equal(t, got.Data, updatedResource.Data)
		})
	}
}

func TestReconcileConfig_ReadinessProbe(t *testing.T) {
	tests := []struct {
		name        string
		runtimeObjs []runtime.Object
		ent         entv1beta1.EnterpriseSearch
		wantCmd     string
	}{
		{
			name:        "create default readiness probe script (no es association)",
			runtimeObjs: nil,
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
			},
			wantCmd: `curl -o /dev/null -w "%{http_code}" https://127.0.0.1:3002/api/ent/v1/internal/health  -k -s --max-time ${READINESS_PROBE_TIMEOUT}`, // no ES basic auth
		},
		{
			name: "update existing readiness probe script if different",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sample-ent-config",
					},
					Data: map[string][]byte{
						ReadinessProbeFilename: []byte("to update"),
					},
				},
			},
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
			},
			wantCmd: `curl -o /dev/null -w "%{http_code}" https://127.0.0.1:3002/api/ent/v1/internal/health  -k -s --max-time ${READINESS_PROBE_TIMEOUT}`, // no ES basic auth
		},
		{
			name: "with ES association: use ES user credentials",
			ent: entWithAssociation("sample", commonv1.AssociationConf{
				AuthSecretName: "sample-ent-user",
				AuthSecretKey:  "ns-sample-ent-user",
				CACertProvided: true,
				CASecretName:   "sample-ent-es-ca",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			}),
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sample-ent-user",
					},
					Data: map[string][]byte{
						"ns-sample-ent-user": []byte("password"),
					},
				},
			},
			wantCmd: `curl -o /dev/null -w "%{http_code}" https://127.0.0.1:3002/api/ent/v1/internal/health -u ns-sample-ent-user:password -k -s --max-time ${READINESS_PROBE_TIMEOUT}`,
		},
		{
			name: "with es credentials in a user-provided config secret",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "my-config",
					},
					Data: map[string][]byte{
						"enterprise-search.yml": []byte("elasticsearch.password: mypassword\nelasticsearch.username: myusername"),
					},
				},
			},
			ent: entv1beta1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sample",
				},
				Spec: entv1beta1.EnterpriseSearchSpec{
					ConfigRef: &commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{SecretName: "my-config"},
					},
				},
			},
			wantCmd: `curl -o /dev/null -w "%{http_code}" https://127.0.0.1:3002/api/ent/v1/internal/health -u myusername:mypassword -k -s --max-time ${READINESS_PROBE_TIMEOUT}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &ReconcileEnterpriseSearch{
				Client:         k8s.WrappedFakeClient(tt.runtimeObjs...),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
			}

			got, err := ReconcileConfig(driver, tt.ent)
			require.NoError(t, err)

			require.Contains(t, string(got.Data[ReadinessProbeFilename]), tt.wantCmd)

			var updatedResource corev1.Secret
			err = driver.K8sClient().Get(k8s.ExtractNamespacedName(&got), &updatedResource)
			assert.NoError(t, err)
			assert.Equal(t, got.Data, updatedResource.Data)
		})
	}
}
