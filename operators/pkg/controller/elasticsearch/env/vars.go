// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package env

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	corev1 "k8s.io/api/core/v1"
)

// DynamicPodEnvVars are environment variables to dynamically inject pod name and IP,
// to be referenced in Elasticsearch configuration file
var DynamicPodEnvVars = []corev1.EnvVar{
	{Name: settings.EnvPodName, Value: "", ValueFrom: &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
	}},
	{Name: settings.EnvPodIP, Value: "", ValueFrom: &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
	}},
}
