// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func Test_imageWithVersion(t *testing.T) {
	type args struct {
		image   string
		version string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{image: "someimage", version: "6.4.2"},
			want: "someimage:6.4.2",
		},
		{
			args: args{image: "differentimage", version: "6.4.1"},
			want: "differentimage:6.4.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageWithVersion(tt.args.image, tt.args.version)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewPodTemplateSpec(t *testing.T) {
	tests := []struct {
		name       string
		kb         v1alpha1.Kibana
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "defaults",
			kb: v1alpha1.Kibana{
				Spec: v1alpha1.KibanaSpec{
					Version: "7.1.0",
				},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, false, *pod.Spec.AutomountServiceAccountToken)
				assert.Len(t, pod.Spec.Containers, 1)
				assert.Len(t, pod.Spec.InitContainers, 0)
				kibanaContainer := GetKibanaContainer(pod.Spec)
				require.NotNil(t, kibanaContainer)
				assert.Equal(t, imageWithVersion(defaultImageRepositoryAndName, "7.1.0"), kibanaContainer.Image)
				assert.NotNil(t, kibanaContainer.ReadinessProbe)
				assert.Equal(t, DefaultResources, kibanaContainer.Resources)
				assert.NotEmpty(t, kibanaContainer.Ports)
			},
		},
		{
			name: "with custom image",
			kb: v1alpha1.Kibana{Spec: v1alpha1.KibanaSpec{
				Image:   "my-custom-image:1.0.0",
				Version: "7.1.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, "my-custom-image:1.0.0", GetKibanaContainer(pod.Spec).Image)
			},
		},
		{
			name: "with user-provided init containers",
			kb: v1alpha1.Kibana{Spec: v1alpha1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								Name: "user-init-container",
							},
						},
					},
				},
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
			},
		},
		{
			name: "with user-provided environment",
			kb: v1alpha1.Kibana{Spec: v1alpha1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: v1alpha1.KibanaContainerName,
								Env: []corev1.EnvVar{
									{
										Name:  "user-env",
										Value: "user-env-value",
									},
								},
							},
						},
					},
				},
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, GetKibanaContainer(pod.Spec).Env, 1)
			},
		},
		{
			name: "with user-provided volumes and volume mounts",
			kb: v1alpha1.Kibana{Spec: v1alpha1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: v1alpha1.KibanaContainerName,
								VolumeMounts: []corev1.VolumeMount{
									{
										Name: "user-volume-mount",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "user-volume",
							},
						},
					},
				},
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.Volumes, 1)
				assert.Len(t, GetKibanaContainer(pod.Spec).VolumeMounts, 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPodTemplateSpec(tt.kb)
			tt.assertions(got)
		})
	}
}
