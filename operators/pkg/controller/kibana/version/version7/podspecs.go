// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana/pod"
	corev1 "k8s.io/api/core/v1"
)

const ElasticsearchHosts = "ELASTICSEARCH_HOSTS"

// NewEnv returns environment variables for a 7.x Kibana.
func NewEnv(p pod.SpecParams) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: ElasticsearchHosts, Value: p.ElasticsearchUrl},
	}
	return pod.ApplyToEnv(p.User, env)
}

// NewPodSpec returns a podspec for a 7.x Kibana.
func NewPodSpec(p pod.SpecParams) corev1.PodSpec {
	return pod.NewSpec(p, pod.EnvFactory(NewEnv))
}
