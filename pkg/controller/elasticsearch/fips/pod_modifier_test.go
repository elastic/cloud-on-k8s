// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fips

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
)

func TestInjectKeystorePassword(t *testing.T) {
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
	}{
		{
			name: "inject into empty template",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: keystore.InitContainerName},
					},
				},
			},
		},
		{
			name: "preserve existing volumes and mounts",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "existing-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "existing-volume", MountPath: "/existing"},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: keystore.InitContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "existing-volume", MountPath: "/existing"},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := defaults.NewPodTemplateBuilder(tt.podTemplate, esv1.ElasticsearchContainerName)
			builder = InjectKeystorePassword(builder, "es-es-fips-keystore-password")

			var injectedVolume *corev1.Volume
			for i := range builder.PodTemplate.Spec.Volumes {
				if builder.PodTemplate.Spec.Volumes[i].Name == VolumeName {
					injectedVolume = &builder.PodTemplate.Spec.Volumes[i]
					break
				}
			}
			require.NotNil(t, injectedVolume)
			require.NotNil(t, injectedVolume.Secret)
			require.Equal(t, "es-es-fips-keystore-password", injectedVolume.Secret.SecretName)

			mainContainer := builder.MainContainer()
			require.NotNil(t, mainContainer)
			require.Contains(t, mainContainer.VolumeMounts, corev1.VolumeMount{
				Name:      VolumeName,
				MountPath: MountPath,
				ReadOnly:  true,
			})
			require.Contains(t, mainContainer.Env, corev1.EnvVar{
				Name:  "ES_KEYSTORE_PASSPHRASE_FILE",
				Value: PasswordFile,
			})

			var keystoreInitContainer *corev1.Container
			for i := range builder.PodTemplate.Spec.InitContainers {
				if builder.PodTemplate.Spec.InitContainers[i].Name == keystore.InitContainerName {
					keystoreInitContainer = &builder.PodTemplate.Spec.InitContainers[i]
					break
				}
			}
			require.NotNil(t, keystoreInitContainer)
			require.Contains(t, keystoreInitContainer.VolumeMounts, corev1.VolumeMount{
				Name:      VolumeName,
				MountPath: MountPath,
				ReadOnly:  true,
			})
		})
	}
}
