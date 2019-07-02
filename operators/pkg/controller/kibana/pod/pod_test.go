// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/association/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		keystore   *keystore.Resources
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "defaults",
			kb: v1alpha1.Kibana{
				Spec: v1alpha1.KibanaSpec{
					Version: "7.1.0",
				},
			},
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, false, *pod.Spec.AutomountServiceAccountToken)
				assert.Len(t, pod.Spec.Containers, 1)
				assert.Len(t, pod.Spec.InitContainers, 0)
				assert.Len(t, pod.Spec.Volumes, 1)
				kibanaContainer := GetKibanaContainer(pod.Spec)
				require.NotNil(t, kibanaContainer)
				assert.Equal(t, 1, len(kibanaContainer.VolumeMounts))
				assert.Equal(t, imageWithVersion(defaultImageRepositoryAndName, "7.1.0"), kibanaContainer.Image)
				assert.NotNil(t, kibanaContainer.ReadinessProbe)
				assert.NotEmpty(t, kibanaContainer.Ports)
			},
		},
		{
			name: "with additional volumes and init containers for the Keystore",
			kb: v1alpha1.Kibana{
				Spec: v1alpha1.KibanaSpec{
					Version: "7.1.0",
				},
			},
			keystore: &keystore.Resources{
				KeystoreInitContainer: corev1.Container{Name: "init"},
				KeystoreVolume:        corev1.Volume{Name: "vol"},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Len(t, pod.Spec.Volumes, 2)
			},
		},
		{
			name: "with custom image",
			kb: v1alpha1.Kibana{Spec: v1alpha1.KibanaSpec{
				Image:   "my-custom-image:1.0.0",
				Version: "7.1.0",
			}},
			keystore: nil,
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
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
			},
		},
		{
			name:     "with user-provided labels",
			keystore: nil,
			kb: v1alpha1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kibana-name",
				},
				Spec: v1alpha1.KibanaSpec{
					PodTemplate: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"label1":                  "value1",
								"label2":                  "value2",
								label.KibanaNameLabelName: "overridden-kibana-name",
							},
						},
					},
				}},
			assertions: func(pod corev1.PodTemplateSpec) {
				labels := label.NewLabels("kibana-name")
				labels["label1"] = "value1"
				labels["label2"] = "value2"
				labels[label.KibanaNameLabelName] = "overridden-kibana-name"
				assert.Equal(t, labels, pod.Labels)
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
				assert.Len(t, pod.Spec.Volumes, 2)
				assert.Len(t, GetKibanaContainer(pod.Spec).VolumeMounts, 2)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPodTemplateSpec(tt.kb, tt.keystore)
			tt.assertions(got)
		})
	}
}
