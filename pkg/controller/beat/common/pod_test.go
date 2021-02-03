// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"crypto/sha256"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/stretchr/testify/assert"
)

func Test_buildPodTemplate(t *testing.T) {
	tests := []struct {
		name       string
		beat       v1beta1.Beat
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "deployment user-provided init containers should inherit from the default main container image",
			beat: v1beta1.Beat{Spec: v1beta1.BeatSpec{
				Version: "7.8.0",
				Deployment: &v1beta1.DeploymentSpec{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "user-init-container",
								},
							},
						},
					},
				},
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
			},
		},
		{
			name: "daemonset user-provided init containers should inherit from the default main container image",
			beat: v1beta1.Beat{Spec: v1beta1.BeatSpec{
				Version: "7.8.0",
				DaemonSet: &v1beta1.DaemonSetSpec{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "user-init-container",
								},
							},
						},
					},
				},
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := DriverParams{Beat: tt.beat}
			got := buildPodTemplate(params, container.AuditbeatImage, nil, sha256.New224())
			tt.assertions(got)
		})
	}
}
