// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/env"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

// NewEnvironmentVars returns the environment vars to be associated to a pod
func NewEnvironmentVars(
	p pod.NewPodSpecParams,
) []corev1.EnvVar {
	vars := []corev1.EnvVar{
		{Name: settings.EnvReadinessProbeProtocol, Value: "https"},
		{Name: settings.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: settings.EnvProbePasswordFile, Value: path.Join(esvolume.ProbeUserSecretMountPath, p.ProbeUser.Name)},
	}
	vars = append(vars, env.DynamicPodEnvVars...)

	return vars
}
