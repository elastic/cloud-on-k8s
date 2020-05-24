// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

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
		})

	defaultConfig := settings.MustParseConfig([]byte("default: true"))
	userConfig := &commonv1.Config{Data: map[string]interface{}{"user": "true"}}
	userCanonicalConfig := settings.MustCanonicalConfig(userConfig.Data)
	outputCAYaml := settings.MustParseConfig([]byte("output.elasticsearch.ssl.certificate_authorities: /mnt/elastic-internal/es-certs/ca.crt"))
	outputYaml := settings.MustParseConfig([]byte(`output:
  elasticsearch:
    hosts:
    - url
    password: "123"
    username: elastic
`))

	withAssociation := &beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
		},
	}
	withAssociation.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret",
		AuthSecretKey:  "elastic",
		CACertProvided: false,
		CASecretName:   "secret2",
		URL:            "url",
	})
	withAssociationWithCA := withAssociation.DeepCopy()
	withAssociationWithCA.AssociationConf().CACertProvided = true

	merge := func(cs ...*settings.CanonicalConfig) *settings.CanonicalConfig {
		result := settings.NewCanonicalConfig()
		_ = result.MergeWith(cs...)
		return result
	}

	for _, tt := range []struct {
		name          string
		client        k8s.Client
		associated    commonv1.Associated
		defaultConfig *settings.CanonicalConfig
		userConfig    *commonv1.Config
		want          *settings.CanonicalConfig
		wantErr       bool
	}{
		{
			name:    "neither default nor user config",
			wantErr: true,
		},
		{
			name:          "no association, only default config",
			associated:    &beatv1beta1.Beat{},
			defaultConfig: defaultConfig,
			want:          defaultConfig,
		},
		{
			name:       "no association, only user config",
			associated: &beatv1beta1.Beat{},
			userConfig: userConfig,
			want:       userCanonicalConfig,
		},
		{
			name:          "no association, default and user config",
			associated:    &beatv1beta1.Beat{},
			defaultConfig: defaultConfig,
			userConfig:    userConfig,
			want:          userCanonicalConfig,
		},
		{
			name:          "association without ca, only default config",
			client:        clientWithSecret,
			associated:    withAssociation,
			defaultConfig: defaultConfig,
			want:          merge(defaultConfig, outputYaml),
		},
		{
			name:       "association without ca, only user config",
			client:     clientWithSecret,
			associated: withAssociation,
			userConfig: userConfig,
			want:       merge(userCanonicalConfig, outputYaml),
		},
		{
			name:          "association without ca, default and user config",
			client:        clientWithSecret,
			associated:    withAssociation,
			defaultConfig: defaultConfig,
			userConfig:    userConfig,
			want:          merge(userCanonicalConfig, outputYaml),
		},
		{
			name:          "association with ca, only default config",
			client:        clientWithSecret,
			associated:    withAssociationWithCA,
			defaultConfig: defaultConfig,
			want:          merge(defaultConfig, outputYaml, outputCAYaml),
		},
		{
			name:       "association with ca, only user config",
			client:     clientWithSecret,
			associated: withAssociationWithCA,
			userConfig: userConfig,
			want:       merge(userCanonicalConfig, outputYaml, outputCAYaml),
		},
		{
			name:          "association with ca, default and user config",
			client:        clientWithSecret,
			associated:    withAssociationWithCA,
			defaultConfig: defaultConfig,
			userConfig:    userConfig,
			want:          merge(userCanonicalConfig, outputYaml, outputCAYaml),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotYaml, gotErr := buildBeatConfig(tt.client, tt.associated, tt.defaultConfig, tt.userConfig)

			diff := tt.want.Diff(settings.MustParseConfig(gotYaml), nil)

			require.Empty(t, diff)
			require.Equal(t, gotErr != nil, tt.wantErr)
		})
	}
}
