// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana/pod"
	"github.com/magiconair/properties/assert"
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
		args       pod.SpecParams
		assertions func(corev1.PodSpec)
	}{
		{
			name: "happy path",
			args: pod.SpecParams{
				Version:          "6.6.1",
				ElasticsearchUrl: "http://localhost:9200",
				CustomImageName:  "img",
				User:             v1alpha1.ElasticsearchAuth{},
			},
			assertions: func(spec corev1.PodSpec) {
				url := envWithName(t, ElasticsearchURL, spec.Containers[0])
				assert.Equal(t, url.Value, "http://localhost:9200")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assertions(NewPodSpec(tt.args))
		})
	}
}
