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

	defaultConfig := settings.MustParseConfig([]byte("default: true"))
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

	merge := func(cs ...*settings.CanonicalConfig) *settings.CanonicalConfig {
		result := settings.NewCanonicalConfig()
		_ = result.MergeWith(cs...)
		return result
	}

	for _, tt := range []struct {
		name          string
		client        k8s.Client
		beat          beatv1beta1.Beat
		defaultConfig *settings.CanonicalConfig
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
			defaultConfig: defaultConfig,
			want:          defaultConfig,
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
			defaultConfig: defaultConfig,
			want:          userCanonicalConfig,
		},
		{
			name:          "association without ca, only default config",
			client:        clientWithSecret,
			beat:          withAssociation,
			defaultConfig: defaultConfig,
			want:          merge(defaultConfig, outputYaml),
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
			defaultConfig: defaultConfig,
			want:          merge(userCanonicalConfig, outputYaml),
		},
		{
			name:          "association with ca, only default config",
			client:        clientWithSecret,
			beat:          withAssociationWithCA,
			defaultConfig: defaultConfig,
			want:          merge(defaultConfig, outputYaml, outputCAYaml),
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
			defaultConfig: defaultConfig,
			want:          merge(userCanonicalConfig, outputYaml, outputCAYaml),
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
