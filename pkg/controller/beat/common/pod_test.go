// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
		name string
		beat v1beta1.Beat
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := DriverParams{Beat: tt.beat}
			assertPodWithInitContainer(t, buildPodTemplate(params, container.AuditbeatImage, nil, sha256.New224()))
		})
	}
}

// decimal value of '0444' in octal is 292
var expectedConfigVolumeMode int32 = 292

func assertPodWithInitContainer(t *testing.T, pod corev1.PodTemplateSpec) {
	// Validate that init container is in the PodTemplate
	assert.Len(t, pod.Spec.InitContainers, 1)
	// Image used by the init container and by the "main" container must be the same
	assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
	// Validate that the Pod contains a Secret as a config volume.
	var configVolume *corev1.SecretVolumeSource
	for _, vol := range pod.Spec.Volumes {
		if vol.Secret != nil && vol.Name == "config" {
			configVolume = vol.Secret
			break
		}
	}
	assert.NotNil(t, configVolume)
	// Validate the mode
	assert.NotNil(t, configVolume.DefaultMode, "default volume mode for beat configuration should not be nil")
	assert.Equal(t, expectedConfigVolumeMode, *configVolume.DefaultMode)
}
