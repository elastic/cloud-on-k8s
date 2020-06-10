// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	logrtesting "github.com/go-logr/logr/testing"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func merge(cs ...*settings.CanonicalConfig) *settings.CanonicalConfig {
	result := settings.NewCanonicalConfig()
	err := result.MergeWith(cs...)
	if err != nil {
		panic(err)
	}
	return result
}

func Test_buildBeatConfig(t *testing.T) {
	clientWithSecret := k8s.WrappedFakeClient(
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

	unmanagedDefaultCfg := settings.MustParseConfig([]byte("default: true"))
	managedDefaultCfg := settings.MustParseConfig([]byte("setup.kibana: true"))
	userConfig := &commonv1.Config{Data: map[string]interface{}{"user": "true"}}
	userCanonicalConfig := settings.MustCanonicalConfig(userConfig.Data)
	outputCAYaml := settings.MustParseConfig([]byte(`output.elasticsearch.ssl.certificate_authorities: 
   - /mnt/elastic-internal/elasticsearch-certs/ca.crt`))
	outputYaml := settings.MustParseConfig([]byte(`output:
  elasticsearch:
    hosts:
    - url
    password: "123"
    username: elastic
`))

	withAssociation := beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
		},
	}
	esAssoc := beatv1beta1.BeatESAssociation{Beat: &withAssociation}
	esAssoc.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret",
		AuthSecretKey:  "elastic",
		CACertProvided: false,
		CASecretName:   "secret2",
		URL:            "url",
	})
	withAssociationWithCA := *withAssociation.DeepCopy()

	esAssocWithCA := beatv1beta1.BeatESAssociation{Beat: &withAssociationWithCA}
	esAssocWithCA.AssociationConf().CACertProvided = true

	withAssociationWithCAAndConfig := *withAssociationWithCA.DeepCopy()
	withAssociationWithCAAndConfig.Spec.Config = userConfig

	withAssociationWithConfig := *withAssociation.DeepCopy()
	withAssociationWithConfig.Spec.Config = userConfig

	for _, tt := range []struct {
		name          string
		client        k8s.Client
		beat          beatv1beta1.Beat
		defaultConfig DefaultConfig
		want          *settings.CanonicalConfig
		wantErr       bool
	}{
		{
			name: "neither default nor user config",
			beat: beatv1beta1.Beat{},
		},
		{
			name:          "no association, only default config",
			beat:          beatv1beta1.Beat{},
			defaultConfig: DefaultConfig{Unmanaged: unmanagedDefaultCfg},
			want:          unmanagedDefaultCfg,
		},
		{
			name: "no association, only user config",
			beat: beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{
				Config: userConfig,
			}},
			want: userCanonicalConfig,
		},
		{
			name: "no association, default and user config",
			beat: beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{
				Config: userConfig,
			}},
			defaultConfig: DefaultConfig{Unmanaged: unmanagedDefaultCfg},
			want:          userCanonicalConfig,
		},
		{
			name: "no association, both default configs and user config",
			beat: beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{
				Config: userConfig,
			}},
			defaultConfig: DefaultConfig{
				Unmanaged: unmanagedDefaultCfg,
				Managed:   managedDefaultCfg,
			},
			want: merge(userCanonicalConfig, managedDefaultCfg),
		},

		{
			name:          "association without ca, only default config",
			client:        clientWithSecret,
			beat:          withAssociation,
			defaultConfig: DefaultConfig{Unmanaged: unmanagedDefaultCfg},
			want:          merge(unmanagedDefaultCfg, outputYaml),
		},
		{
			name:   "association without ca, only user config",
			client: clientWithSecret,
			beat:   withAssociationWithConfig,
			want:   merge(userCanonicalConfig, outputYaml),
		},
		{
			name:          "association without ca, default and user config",
			client:        clientWithSecret,
			beat:          withAssociationWithConfig,
			defaultConfig: DefaultConfig{Unmanaged: unmanagedDefaultCfg},
			want:          merge(userCanonicalConfig, outputYaml),
		},
		{
			name:          "association with ca, only default config",
			client:        clientWithSecret,
			beat:          withAssociationWithCA,
			defaultConfig: DefaultConfig{Unmanaged: unmanagedDefaultCfg},
			want:          merge(unmanagedDefaultCfg, outputYaml, outputCAYaml),
		},
		{
			name:   "association with ca, only user config",
			client: clientWithSecret,
			beat:   withAssociationWithCAAndConfig,
			want:   merge(userCanonicalConfig, outputYaml, outputCAYaml),
		},
		{
			name:          "association with ca, default and user config",
			client:        clientWithSecret,
			beat:          withAssociationWithCAAndConfig,
			defaultConfig: DefaultConfig{Unmanaged: unmanagedDefaultCfg},
			want:          merge(userCanonicalConfig, outputYaml, outputCAYaml),
		},
		{
			name:   "association with ca, both default configs and user config",
			client: clientWithSecret,
			beat:   withAssociationWithCAAndConfig,
			defaultConfig: DefaultConfig{
				Unmanaged: unmanagedDefaultCfg,
				Managed:   managedDefaultCfg,
			},
			want: merge(userCanonicalConfig, managedDefaultCfg, outputYaml, outputCAYaml),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotYaml, gotErr := buildBeatConfig(logrtesting.NullLogger{}, tt.client, tt.beat, tt.defaultConfig)

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
				client:     k8s.WrappedFakeClient(),
				associated: associationFixture(kibanaAssocConf),
			},
			wantErr: true,
		},
		{
			name: "association: no ca",
			args: args{
				client:     k8s.WrappedFakeClient(secretFixture),
				associated: associationFixture(kibanaAssocConf),
			},
			want:    expectedConfig,
			wantErr: false,
		},
		{
			name: "association: with ca",
			args: args{
				client:     k8s.WrappedFakeClient(secretFixture),
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
