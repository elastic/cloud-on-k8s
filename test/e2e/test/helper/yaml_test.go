// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestTweakEnvSecretRefs(t *testing.T) {
	fileSecrets := sets.New("apm-secret-token", "other-secret")

	envVar := func(name, secretName, secretKey string) corev1.EnvVar {
		return corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  secretKey,
				},
			},
		}
	}
	plainEnvVar := func(name, value string) corev1.EnvVar {
		return corev1.EnvVar{Name: name, Value: value}
	}

	for _, tc := range []struct {
		name          string
		podSpec       corev1.PodSpec
		wantEnvs      map[string]string // container name → secretKeyRef name after tweak
		wantUnchanged []string          // env var names whose secretKeyRef must not change
	}{
		{
			name: "suffixes file-owned secret refs in containers",
			podSpec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "kibana", Env: []corev1.EnvVar{
						envVar("APM_SECRET_TOKEN", "apm-secret-token", "token"),
					}},
				},
			},
			wantEnvs: map[string]string{"APM_SECRET_TOKEN": "apm-secret-token-abc"},
		},
		{
			name: "does not suffix ECK-managed secret refs not in fileSecrets",
			podSpec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "metricbeat", Env: []corev1.EnvVar{
						envVar("MONITORED_ES_PASSWORD", "elasticsearch-es-elastic-user", "elastic"),
					}},
				},
			},
			wantUnchanged: []string{"MONITORED_ES_PASSWORD"},
		},
		{
			name: "handles mixed env vars: file secret suffixed, ECK secret unchanged, plain value unchanged",
			podSpec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c", Env: []corev1.EnvVar{
						envVar("FILE_SECRET", "apm-secret-token", "token"),
						envVar("ECK_SECRET", "elasticsearch-es-elastic-user", "elastic"),
						plainEnvVar("PLAIN", "value"),
					}},
				},
			},
			wantEnvs:      map[string]string{"FILE_SECRET": "apm-secret-token-abc"},
			wantUnchanged: []string{"ECK_SECRET"},
		},
		{
			name: "suffixes file-owned secret refs in init containers",
			podSpec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init", Env: []corev1.EnvVar{
						envVar("TOKEN", "other-secret", "key"),
					}},
				},
			},
			wantEnvs: map[string]string{"TOKEN": "other-secret-abc"},
		},
		{
			name:    "no-op on empty pod spec",
			podSpec: corev1.PodSpec{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tweakEnvSecretRefs(&tc.podSpec, "abc", fileSecrets)

			allContainers := make([]corev1.Container, 0, len(tc.podSpec.InitContainers)+len(tc.podSpec.Containers))
			allContainers = append(allContainers, tc.podSpec.InitContainers...)
			allContainers = append(allContainers, tc.podSpec.Containers...)

			for _, c := range allContainers {
				for _, env := range c.Env {
					if want, ok := tc.wantEnvs[env.Name]; ok {
						assert.Equal(t, want, env.ValueFrom.SecretKeyRef.Name,
							"env var %s secretKeyRef.Name", env.Name)
					}
					for _, unchanged := range tc.wantUnchanged {
						if env.Name == unchanged {
							original := env.ValueFrom.SecretKeyRef.Name
							assert.NotContains(t, original, "-abc",
								"env var %s should not have been suffixed", env.Name)
						}
					}
				}
			}
		})
	}
}
