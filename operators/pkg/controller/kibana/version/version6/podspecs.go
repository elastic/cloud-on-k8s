// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	corev1 "k8s.io/api/core/v1"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/pod"
)

const ElasticsearchURL = "ELASTICSEARCH_URL"

// NewEnv returns environment variables for a 6.x Kibana.
func NewEnv(kibana kbtype.Kibana) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: ElasticsearchURL, Value: kibana.Spec.Elasticsearch.URL},
	}
	return pod.ApplyToEnv(kibana.Spec.Elasticsearch.Auth, env)
}

// NewPodTemplateSpec returns a pod spec for a 6.x Kibana.
func NewPodTemplateSpec(kibana kbtype.Kibana) corev1.PodTemplateSpec {
	return pod.NewPodTemplateSpec(kibana, pod.EnvFactory(NewEnv))
}
