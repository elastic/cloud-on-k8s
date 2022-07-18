// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func merge(cs ...*settings.CanonicalConfig) *settings.CanonicalConfig {
	result := settings.NewCanonicalConfig()
	if err := result.MergeWith(cs...); err != nil {
		panic(err)
	}
	return result
}

func Test_buildBeatConfig(t *testing.T) {
	clientWithSecret := k8s.NewFakeClient(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: "ns",
			},
			Data: map[string][]byte{"elastic": []byte("123")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret2",
				Namespace: "ns",
			},
			Data: map[string][]byte{"elastic": []byte("123")},
		},
	)

	clientWithMonitoringEnabled := k8s.NewFakeClient(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: "ns",
			},
			Data: map[string][]byte{"elastic": []byte("123")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-user-secret",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"elastic-external": []byte("asdf"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-es-monitoring",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"url":      []byte("https://external-es.external.com"),
				"username": []byte("monitoring-user"),
				"password": []byte("asdfasdf"),
				"ca.crt":   []byte("my_pem_encoded_cert"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testbeat-es-testes-ns-monitoring-ca",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"ca.crt": []byte("my_pem_encoded_cert"),
			},
		},
		&esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testes",
				Namespace: "ns",
			},
		},
	)

	managedCfg := settings.MustParseConfig([]byte("setup.kibana: true"))
	userCfg := &commonv1.Config{Data: map[string]interface{}{"user": "true"}}
	userCanonicalCfg := settings.MustCanonicalConfig(userCfg.Data)
	outputCAYaml := settings.MustParseConfig([]byte(`output.elasticsearch.ssl.certificate_authorities:
   - /mnt/elastic-internal/elasticsearch-certs/ca.crt`))
	outputYaml := settings.MustParseConfig([]byte(`output:
  elasticsearch:
    hosts:
    - url
    password: "123"
    username: elastic
`))
	monitoringYaml := settings.MustParseConfig([]byte(`monitoring:
  enabled: true
  elasticsearch:
    hosts:
    - "https://testes-es-internal-http.ns.svc:9200"
    username: elastic
    password: "123"
    ssl:
      certificate_authorities:
        - "/mnt/elastic-internal/beat-monitoring-certs/ca.crt"
      verification_mode: "certificate"
`))
	externalMonitoringYaml := settings.MustParseConfig([]byte(`monitoring:
  enabled: true
  elasticsearch:
    hosts:
    - "https://external-es.external.com"
    username: "monitoring-user"
    password: asdfasdf
    ssl:
      certificate_authorities:
        - "/mnt/elastic-internal/beat-monitoring-certs/ca.crt"
      verification_mode: "certificate"
`))

	withAssoc := beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
		},
	}
	esAssoc := beatv1beta1.BeatESAssociation{Beat: &withAssoc}
	esAssoc.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret",
		AuthSecretKey:  "elastic",
		CACertProvided: false,
		CASecretName:   "secret2",
		URL:            "url",
	})
	withAssocWithCA := *withAssoc.DeepCopy()

	esAssocWithCA := beatv1beta1.BeatESAssociation{Beat: &withAssocWithCA}
	esAssocWithCA.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret",
		AuthSecretKey:  "elastic",
		CACertProvided: true,
		CASecretName:   "secret2",
		URL:            "url",
	})
	assocConf, err := esAssocWithCA.AssociationConf()
	require.NoError(t, err)
	assocConf.CACertProvided = true

	withAssocWithCAWithonfig := *withAssocWithCA.DeepCopy()
	withAssocWithCAWithonfig.Spec.Config = userCfg

	withAssocWithConfig := *withAssoc.DeepCopy()
	withAssocWithConfig.Spec.Config = userCfg

	for _, tt := range []struct {
		name          string
		client        k8s.Client
		beat          func() beatv1beta1.Beat
		managedConfig *settings.CanonicalConfig
		want          *settings.CanonicalConfig
		wantErr       bool
	}{
		{
			name: "no association, no configs",
			beat: func() beatv1beta1.Beat { return beatv1beta1.Beat{} },
		},
		{
			name: "no association, user config",
			beat: func() beatv1beta1.Beat {
				return beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{
					Config: userCfg,
				}}
			},
			want: userCanonicalCfg,
		},
		{
			name:   "no association, user config, with monitoring enabled",
			client: clientWithMonitoringEnabled,
			beat: func() beatv1beta1.Beat {
				b := beatv1beta1.Beat{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testbeat",
						Namespace: "ns",
					},
					Spec: beatv1beta1.BeatSpec{
						Config: userCfg,
						Monitoring: beatv1beta1.Monitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:      "testes",
									Namespace: "ns",
								},
							},
						},
					}}
				b.MonitoringAssociation(commonv1.ObjectSelector{Name: "testes", Namespace: "ns"}).SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "secret",
					AuthSecretKey:  "elastic",
					CACertProvided: true,
					CASecretName:   "testbeat-es-testes-ns-monitoring-ca",
					URL:            "https://testes-es-internal-http.ns.svc:9200",
				})
				return b
			},
			want: merge(userCanonicalCfg, monitoringYaml),
		},
		{
			name:   "no association, user config, with monitoring enabled, external es cluster",
			client: clientWithMonitoringEnabled,
			beat: func() beatv1beta1.Beat {
				b := beatv1beta1.Beat{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testbeat",
						Namespace: "ns",
					},
					Spec: beatv1beta1.BeatSpec{
						Config: userCfg,
						Monitoring: beatv1beta1.Monitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									SecretName: "external-es-monitoring",
									Namespace:  "ns",
								},
							},
						},
					}}
				b.MonitoringAssociation(commonv1.ObjectSelector{SecretName: "external-es-monitoring", Namespace: "ns"}).SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "external-es-monitoring",
					AuthSecretKey:  "password",
					CASecretName:   "testbeat-es-testes-ns-monitoring-ca",
					CACertProvided: true,
					URL:            "https://external-es.external.com",
				})
				return b
			},
			want: merge(userCanonicalCfg, externalMonitoringYaml),
		},
		{
			name:          "no association, managed config",
			beat:          func() beatv1beta1.Beat { return beatv1beta1.Beat{} },
			managedConfig: managedCfg,
			want:          managedCfg,
		},
		{
			name: "no association, managed and user configs",
			beat: func() beatv1beta1.Beat {
				return beatv1beta1.Beat{
					Spec: beatv1beta1.BeatSpec{
						Config: userCfg,
					},
				}
			},
			managedConfig: managedCfg,
			want:          merge(userCanonicalCfg, managedCfg),
		},
		{
			name:   "association without ca, no configs",
			client: clientWithSecret,
			beat:   func() beatv1beta1.Beat { return withAssoc },
			want:   outputYaml,
		},
		{
			name:   "association without ca, user config",
			client: clientWithSecret,
			beat:   func() beatv1beta1.Beat { return withAssocWithConfig },
			want:   merge(userCanonicalCfg, outputYaml),
		},
		{
			name:          "association without ca, managed config",
			client:        clientWithSecret,
			beat:          func() beatv1beta1.Beat { return withAssoc },
			managedConfig: managedCfg,
			want:          merge(managedCfg, outputYaml),
		},
		{
			name:          "association without ca, user and managed configs",
			client:        clientWithSecret,
			beat:          func() beatv1beta1.Beat { return withAssocWithConfig },
			managedConfig: managedCfg,
			want:          merge(userCanonicalCfg, managedCfg, outputYaml),
		},
		{
			name:   "association with ca, no configs",
			client: clientWithSecret,
			beat:   func() beatv1beta1.Beat { return withAssocWithCA },
			want:   merge(outputYaml, outputCAYaml),
		},
		{
			name:   "association with ca, user config",
			client: clientWithSecret,
			beat:   func() beatv1beta1.Beat { return withAssocWithCAWithonfig },
			want:   merge(userCanonicalCfg, outputYaml, outputCAYaml),
		},
		{
			name:          "association with ca, managed config",
			client:        clientWithSecret,
			beat:          func() beatv1beta1.Beat { return withAssocWithCA },
			managedConfig: managedCfg,
			want:          merge(managedCfg, outputYaml, outputCAYaml),
		},
		{
			name:          "association with ca, user and managed configs",
			client:        clientWithSecret,
			beat:          func() beatv1beta1.Beat { return withAssocWithCAWithonfig },
			managedConfig: managedCfg,
			want:          merge(userCanonicalCfg, managedCfg, outputYaml, outputCAYaml),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotYaml, gotErr := buildBeatConfig(DriverParams{
				Client:        tt.client,
				Context:       nil,
				Logger:        logr.Discard(),
				Watches:       watches.NewDynamicWatches(),
				EventRecorder: nil,
				Beat:          tt.beat(),
			}, tt.managedConfig)

			diff := tt.want.Diff(settings.MustParseConfig(gotYaml), nil)

			if len(diff) != 0 {
				wantBytes, _ := tt.want.Render()
				t.Errorf("buildBeatConfig() got unexpected differences: %s", cmp.Diff(string(wantBytes), string(gotYaml)))
			}
			require.Equal(t, gotErr != nil, tt.wantErr)
		})
	}
}

func TestBuildKibanaConfig(t *testing.T) {
	secretFixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auth-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{"elastic": []byte("123")},
	}
	kibanaAssocConf := commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "elastic",
		CACertProvided: false,
		CASecretName:   "ca-secret",
		URL:            "url",
	}

	kibanaAssocConfWithCA := kibanaAssocConf
	kibanaAssocConfWithCA.CACertProvided = true

	associationFixture := func(conf commonv1.AssociationConf) beatv1beta1.BeatKibanaAssociation {
		assoc := beatv1beta1.BeatKibanaAssociation{Beat: &beatv1beta1.Beat{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "beat",
				Namespace: "test-ns",
			},
			Spec: beatv1beta1.BeatSpec{
				KibanaRef: commonv1.ObjectSelector{
					Name:      "auth-secret",
					Namespace: "test-ns",
				}}}}

		assoc.SetAssociationConf(&conf)
		return assoc
	}

	expectedConfig := settings.MustParseConfig([]byte(`setup.dashboards.enabled: true
setup.kibana: 
  host: url
  username: elastic
  password: "123"
`))

	expectedCAConfig := settings.MustParseConfig([]byte(`setup.kibana.ssl.certificate_authorities: 
  - "/mnt/elastic-internal/kibana-certs/ca.crt"
`))

	type args struct {
		client     k8s.Client
		associated beatv1beta1.BeatKibanaAssociation
	}
	tests := []struct {
		name    string
		args    args
		want    *settings.CanonicalConfig
		wantErr bool
	}{
		{
			name: "no association",
			args: args{
				associated: beatv1beta1.BeatKibanaAssociation{Beat: &beatv1beta1.Beat{}},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "association: no auth-secret",
			args: args{
				client:     k8s.NewFakeClient(),
				associated: associationFixture(kibanaAssocConf),
			},
			wantErr: true,
		},
		{
			name: "association: no ca",
			args: args{
				client:     k8s.NewFakeClient(secretFixture),
				associated: associationFixture(kibanaAssocConf),
			},
			want:    expectedConfig,
			wantErr: false,
		},
		{
			name: "association: with ca",
			args: args{
				client:     k8s.NewFakeClient(secretFixture),
				associated: associationFixture(kibanaAssocConfWithCA),
			},
			want:    merge(expectedConfig, expectedCAConfig),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildKibanaConfig(tt.args.client, tt.args.associated)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildKibanaConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			diff := tt.want.Diff(got, nil)
			if len(diff) != 0 {
				wantBytes, _ := tt.want.Render()
				gotBytes, _ := got.Render()
				t.Errorf("BuildKibanaConfig() got unexpected differences: %s", cmp.Diff(string(wantBytes), string(gotBytes)))
			}
		})
	}
}

func Test_getUserConfig(t *testing.T) {
	for _, tt := range []struct {
		name      string
		config    *commonv1.Config
		configRef *commonv1.ConfigSource
		client    k8s.Client
		want      *settings.CanonicalConfig
		wantErr   bool
	}{
		{
			name: "no user config",
			want: nil,
		},
		{
			name:   "config populated",
			config: &commonv1.Config{Data: map[string]interface{}{"config": "true"}},
			want:   settings.MustParseConfig([]byte(`config: "true"`)),
		},
		{
			name: "configref populated - no secret",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-config",
				},
			},
			client:  k8s.NewFakeClient(),
			wantErr: true,
		},
		{
			name: "configref populated - no secret key",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-config",
				},
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-secret-config",
				},
			}),
			wantErr: true,
		},
		{
			name: "configref populated - malformed config",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-config-2",
				},
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-secret-config-2",
				},
				Data: map[string][]byte{"beat.yml": []byte("filebeat:bad:value")},
			}),
			wantErr: true,
		},
		{
			name: "configref populated",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-config-2",
				},
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-secret-config-2",
				},
				Data: map[string][]byte{"beat.yml": []byte(`filebeat: "true"`)},
			}),
			want: settings.MustParseConfig([]byte(`filebeat: "true"`)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			params := DriverParams{
				Context:       context.Background(),
				Logger:        logr.Discard(),
				Client:        tt.client,
				EventRecorder: &record.FakeRecorder{},
				Watches:       watches.NewDynamicWatches(),
				Beat: beatv1beta1.Beat{
					Spec: beatv1beta1.BeatSpec{
						Config:    tt.config,
						ConfigRef: tt.configRef,
					},
				},
			}

			got, gotErr := getUserConfig(params)
			require.Equal(t, tt.wantErr, gotErr != nil)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_buildMonitoringConfig(t *testing.T) {
	k8sClient := k8s.NewFakeClient()
	k8sClientWithValidMonitoring := k8s.NewFakeClient(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "beat-secret",
				Namespace: "test",
			},
			Data: map[string][]byte{"elastic": []byte("123")},
		},
	)
	monitoringYaml := settings.MustParseConfig([]byte(`monitoring:
  enabled: true
  elasticsearch:
    hosts:
    - "https://testes-es-internal-http.ns.svc:9200"
    username: elastic
    password: "123"
    ssl:
      certificate_authorities:
        - "/mnt/elastic-internal/beat-monitoring-certs/ca.crt"
      verification_mode: "certificate"
`))
	tests := []struct {
		name              string
		params            func() DriverParams
		want              *settings.CanonicalConfig
		wantErr           bool
		expectedErrString string
	}{
		{
			name: "beat without monitoring.ElasticsearchRefs returns error",
			params: func() DriverParams {
				return DriverParams{
					Beat: beatv1beta1.Beat{
						Spec: beatv1beta1.BeatSpec{
							Monitoring: beatv1beta1.Monitoring{},
						},
					},
				}
			},
			want:              nil,
			wantErr:           true,
			expectedErrString: "ElasticsearchRef must exist when stack monitoring is enabled",
		},
		{
			name: "beat with monitoring.ElasticsearchRef that isn't properly defined returns error",
			params: func() DriverParams {
				return DriverParams{
					Beat: beatv1beta1.Beat{
						Spec: beatv1beta1.BeatSpec{
							Monitoring: beatv1beta1.Monitoring{
								ElasticsearchRefs: []commonv1.ObjectSelector{
									{
										Name:       "",
										SecretName: "",
									},
								},
							},
						},
					},
				}
			},
			want:              nil,
			wantErr:           true,
			expectedErrString: "Beats must be associated to an Elasticsearch cluster through elasticsearchRef in order to enable monitoring metrics features",
		},
		{
			name: "beat with monitoring.ElasticsearchRef with invalid association config (secret not found) returns error",
			params: func() DriverParams {
				beat := beatv1beta1.Beat{
					Spec: beatv1beta1.BeatSpec{
						Monitoring: beatv1beta1.Monitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:       "fake",
									SecretName: "fake",
								},
							},
						},
					},
				}
				beat.MonitoringAssociation(commonv1.ObjectSelector{Name: "fake", SecretName: "fake"}).SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "does-not-exist",
					AuthSecretKey:  "invalid",
				})
				return DriverParams{
					Beat:   beat,
					Client: k8sClient,
				}
			},
			want:              nil,
			wantErr:           true,
			expectedErrString: `secrets "does-not-exist" not found`,
		},
		{
			name: "beat with valid monitoring association configuration succeeds",
			params: func() DriverParams {
				beat := beatv1beta1.Beat{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "beat",
						Namespace: "test",
					},
					Spec: beatv1beta1.BeatSpec{
						Monitoring: beatv1beta1.Monitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:       "fake",
									SecretName: "fake",
								},
							},
						},
					},
				}
				beat.MonitoringAssociation(commonv1.ObjectSelector{Name: "fake", SecretName: "fake"}).SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "beat-secret",
					AuthSecretKey:  "elastic",
					CACertProvided: true,
					URL:            "https://testes-es-internal-http.ns.svc:9200",
				})
				return DriverParams{
					Beat:   beat,
					Client: k8sClientWithValidMonitoring,
				}
			},
			want:    monitoringYaml,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildMonitoringConfig(tt.params())
			if (err != nil) != tt.wantErr {
				t.Errorf("buildMonitoringConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && (tt.expectedErrString != err.Error()) {
				t.Errorf("buildMonitoringConfig() = %v, want %v", err.Error(), tt.expectedErrString)
				return
			}
			if len(tt.want.Diff(got, nil)) != 0 {
				wantBytes, _ := tt.want.Render()
				gotBytes, _ := got.Render()
				t.Errorf("buildMonitoringConfig() got unexpected differences: %s", cmp.Diff(string(wantBytes), string(gotBytes)))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildMonitoringConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
