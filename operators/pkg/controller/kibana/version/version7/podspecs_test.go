// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func envWithName(t *testing.T, name string, container corev1.Container) corev1.EnvVar {
	for _, v := range container.Env {
		if v.Name == name {
			return v
		}
	}
	t.Errorf("expected env var %s does not exist ", name)
	return corev1.EnvVar{}
}

func TestNewPodSpec(t *testing.T) {
	tests := []struct {
		name       string
		kb         v1alpha1.Kibana
		assertions func(corev1.PodTemplateSpec)
	}{
		{
			name: "happy path",
			kb: v1alpha1.Kibana{
				Spec: v1alpha1.KibanaSpec{
					Version: "7.0.0",
					Image:   "img",
					Elasticsearch: v1alpha1.BackendElasticsearch{
						URL: "http://localhost:9200",
					},
				},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				url := envWithName(t, ElasticsearchHosts, pod.Spec.Containers[0])
				assert.Equal(t, url.Value, "http://localhost:9200")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assertions(NewPodTemplateSpec(tt.kb))
		})
	}
}
