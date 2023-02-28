// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_buildPipeline(t *testing.T) {
	defaultPipelinesConfig := MustPipelinesConfig(
		[]map[string]string{
			{
				"pipeline.id":   "demo",
				"config.string": "input { exec { command => \"uptime\" interval => 5 } } output { stdout{} }",
			},
		},
	)

	for _, tt := range []struct {
		name      string
		pipelines []map[string]string
		configRef *commonv1.ConfigSource
		client    k8s.Client
		want      *PipelinesConfig
		wantErr   bool
	}{
		{
			name: "no user pipeline",
			want: defaultPipelinesConfig,
		},
		{
			name:      "pipeline populated",
			pipelines: []map[string]string{{"pipeline.id": "main"}},
			want:      MustParsePipelineConfig([]byte(`- "pipeline.id": "main"`)),
		},
		{
			name: "configref populated - no secret",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-pipeline",
				},
			},
			client:  k8s.NewFakeClient(),
			want:    NewPipelinesConfig(),
			wantErr: true,
		},
		{
			name: "configref populated - no secret key",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-pipeline",
				},
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-secret-pipeline",
				},
			}),
			want:    NewPipelinesConfig(),
			wantErr: true,
		},
		{
			name: "configref populated - malformed config",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-pipeline-2",
				},
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-secret-pipeline-2",
				},
				Data: map[string][]byte{"pipelines.yml": []byte("something:bad:value")},
			}),
			want:    NewPipelinesConfig(),
			wantErr: true,
		},
		{
			name: "configref populated",
			configRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-pipeline-2",
				},
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-secret-pipeline-2",
				},
				Data: map[string][]byte{"pipelines.yml": []byte(`- "pipeline.id": "main"`)},
			}),
			want: MustParsePipelineConfig([]byte(`- "pipeline.id": "main"`)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			params := Params{
				Context:       context.Background(),
				Client:        tt.client,
				EventRecorder: &record.FakeRecorder{},
				Watches:       watches.NewDynamicWatches(),
				Logstash: logstashv1alpha1.Logstash{
					Spec: logstashv1alpha1.LogstashSpec{
						Pipelines:    tt.pipelines,
						PipelinesRef: tt.configRef,
					},
				},
			}

			gotYaml, gotErr := buildPipeline(params)
			diff, err := tt.want.Diff(MustParsePipelineConfig(gotYaml))
			if diff {
				t.Errorf("buildPipeline() got unexpected differences: %v", err)
			}

			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}
