// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
		beat          beatv1beta1.Beat
		managedConfig *settings.CanonicalConfig
		want          *settings.CanonicalConfig
		wantErr       bool
	}{
		{
			name: "no association, no configs",
			beat: beatv1beta1.Beat{},
		},
		{
			name: "no association, user config",
			beat: beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{
				Config: userCfg,
			}},
			want: userCanonicalCfg,
		},
		{
			name:          "no association, managed config",
			beat:          beatv1beta1.Beat{},
			managedConfig: managedCfg,
			want:          managedCfg,
		},
		{
			name: "no association, managed and user configs",
			beat: beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{
				Config: userCfg,
			}},
			managedConfig: managedCfg,
			want:          merge(userCanonicalCfg, managedCfg),
		},
		{
			name:   "association without ca, no configs",
			client: clientWithSecret,
			beat:   withAssoc,
			want:   outputYaml,
		},
		{
			name:   "association without ca, user config",
			client: clientWithSecret,
			beat:   withAssocWithConfig,
			want:   merge(userCanonicalCfg, outputYaml),
		},
		{
			name:          "association without ca, managed config",
			client:        clientWithSecret,
			beat:          withAssoc,
			managedConfig: managedCfg,
			want:          merge(managedCfg, outputYaml),
		},
		{
			name:          "association without ca, user and managed configs",
			client:        clientWithSecret,
			beat:          withAssocWithConfig,
			managedConfig: managedCfg,
			want:          merge(userCanonicalCfg, managedCfg, outputYaml),
		},
		{
			name:   "association with ca, no configs",
			client: clientWithSecret,
			beat:   withAssocWithCA,
			want:   merge(outputYaml, outputCAYaml),
		},
		{
			name:   "association with ca, user config",
			client: clientWithSecret,
			beat:   withAssocWithCAWithonfig,
			want:   merge(userCanonicalCfg, outputYaml, outputCAYaml),
		},
		{
			name:          "association with ca, managed config",
			client:        clientWithSecret,
			beat:          withAssocWithCA,
			managedConfig: managedCfg,
			want:          merge(managedCfg, outputYaml, outputCAYaml),
		},
		{
			name:          "association with ca, user and managed configs",
			client:        clientWithSecret,
			beat:          withAssocWithCAWithonfig,
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
				Beat:          tt.beat,
			}, tt.managedConfig)

			diff := tt.want.Diff(settings.MustParseConfig(gotYaml), nil)

			require.Empty(t, diff)
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
			require.Empty(t, diff)
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
