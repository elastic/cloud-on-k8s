// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"fmt"
	"reflect"
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
	for _, tt := range []struct {
		name         string
		pipelines    []commonv1.Config
		pipelinesRef *commonv1.ConfigSource
		client       k8s.Client
		want         *PipelinesConfig
		wantErr      bool
	}{
		{
			name: "no user pipeline",
			want: defaultPipeline,
		},
		{
			name: "pipeline populated",
			pipelines: []commonv1.Config{
				{Data: map[string]interface{}{"pipeline.id": "main"}},
			},
			want: MustParsePipelineConfig([]byte(`- "pipeline.id": "main"`)),
		},
		{
			name: "pipelinesref populated - no secret",
			pipelinesRef: &commonv1.ConfigSource{
				SecretRef: commonv1.SecretRef{
					SecretName: "my-secret-pipeline",
				},
			},
			client:  k8s.NewFakeClient(),
			want:    NewPipelinesConfig(),
			wantErr: true,
		},
		{
			name: "pipelinesref populated - no secret key",
			pipelinesRef: &commonv1.ConfigSource{
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
			name: "pipelinesref populated - malformed config",
			pipelinesRef: &commonv1.ConfigSource{
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
			name: "pipelinesref populated",
			pipelinesRef: &commonv1.ConfigSource{
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
						PipelinesRef: tt.pipelinesRef,
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

// Diff returns true if the key/value or the sequence of two PipelinesConfig are different
// Use for testing only.
func (c *PipelinesConfig) Diff(c2 *PipelinesConfig) (bool, error) {
	if c == c2 {
		return false, nil
	}
	if c == nil && c2 != nil {
		return true, fmt.Errorf("empty lhs config %s", c2.asUCfg().FlattenedKeys(Options...))
	}
	if c != nil && c2 == nil {
		return true, fmt.Errorf("empty rhs config %s", c.asUCfg().FlattenedKeys(Options...))
	}

	var s []map[string]interface{}
	var s2 []map[string]interface{}
	err := c.asUCfg().Unpack(&s, Options...)
	if err != nil {
		return true, err
	}
	err = c2.asUCfg().Unpack(&s2, Options...)
	if err != nil {
		return true, err
	}

	return diffSlice(s, s2)
}

// diffSlice returns true if the key/value or the sequence of two PipelinesConfig are different
func diffSlice(s1, s2 []map[string]interface{}) (bool, error) {
	if len(s1) != len(s2) {
		return true, fmt.Errorf("array size doesn't match %d, %d", len(s1), len(s2))
	}
	var diff []string
	for i, m := range s1 {
		m2 := s2[i]
		if eq := reflect.DeepEqual(m, m2); !eq {
			diff = append(diff, fmt.Sprintf("%s vs %s, ", m, m2))
		}
	}

	if len(diff) > 0 {
		return true, fmt.Errorf("there are %d differences. %s", len(diff), diff)
	}

	return false, nil
}
