// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana/pod"
	corev1 "k8s.io/api/core/v1"
)

const ElasticsearchURL = "ELASTICSEARCH_URL"

// NewEnv returns environment variables for a 6.x Kibana.
func NewEnv(p pod.SpecParams) []corev1.EnvVar {

	env := []corev1.EnvVar{
		{Name: ElasticsearchURL, Value: p.ElasticsearchUrl},
	}
	return pod.ApplyToEnv(p.User, env)
}

// NewPodSpec returns a pod spec for a 6.x Kibana.
func NewPodSpec(p pod.SpecParams) corev1.PodSpec {
	return pod.NewSpec(p, pod.EnvFactory(NewEnv))
}
