// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	commonvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestNewPodTemplateSpec(t *testing.T) {
	tests := []struct {
		name       string
		kb         kbv1.Kibana
		keystore   *keystore.Resources
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "defaults",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					Version: "7.1.0",
				},
			},
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, false, *pod.Spec.AutomountServiceAccountToken)
				assert.Len(t, pod.Spec.Containers, 1)
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Len(t, pod.Spec.Volumes, 0)
				kibanaContainer := GetKibanaContainer(pod.Spec)
				require.NotNil(t, kibanaContainer)
				assert.Equal(t, 0, len(kibanaContainer.VolumeMounts))
				assert.Equal(t, container.ImageRepository(container.KibanaImage, version.MustParse("7.1.0")), kibanaContainer.Image)
				assert.NotNil(t, kibanaContainer.ReadinessProbe)
				assert.NotEmpty(t, kibanaContainer.Ports)
			},
		},
		{
			name: "with additional volumes and init containers for the Keystore",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					Version: "7.1.0",
				},
			},
			keystore: &keystore.Resources{
				InitContainer: corev1.Container{Name: "init"},
				Volume:        corev1.Volume{Name: "vol"},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 2)
				assert.Len(t, pod.Spec.Volumes, 1)
			},
		},
		{
			name: "with custom image",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Image:   "my-custom-image:1.0.0",
				Version: "7.1.0",
			}},
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, "my-custom-image:1.0.0", GetKibanaContainer(pod.Spec).Image)
			},
		},
		{
			name: "with default resources",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Version: "7.1.0",
			}},
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, DefaultResources, GetKibanaContainer(pod.Spec).Resources)
			},
		},
		{
			name: "with user-provided resources",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Version: "7.1.0",
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
								Resources: corev1.ResourceRequirements{
									Limits: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceMemory: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			}},
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
				}, GetKibanaContainer(pod.Spec).Resources)
			},
		},
		{
			name: "with user-provided init containers",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								Name: "user-init-container",
							},
						},
					},
				},
				Version: "8.12.0",
			}},
			keystore: nil,
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 2)
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[1].Image)
			},
		},
		{
			name:     "with user-provided labels",
			keystore: nil,
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kibana-name",
				},
				Spec: kbv1.KibanaSpec{
					PodTemplate: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"label1":                    "value1",
								"label2":                    "value2",
								kblabel.KibanaNameLabelName: "overridden-kibana-name",
							},
						},
					},
					Version: "7.4.0",
				}},
			assertions: func(pod corev1.PodTemplateSpec) {
				labels := (&kbv1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "kibana-name"}}).GetIdentityLabels()
				labels[kblabel.KibanaVersionLabelName] = "7.4.0"
				labels["label1"] = "value1"
				labels["label2"] = "value2"
				labels[kblabel.KibanaNameLabelName] = "overridden-kibana-name"
				assert.Equal(t, labels, pod.Labels)
			},
		},
		{
			name: "with user-provided environment",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
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
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, GetKibanaContainer(pod.Spec).Env, 1)
			},
		},
		{
			name: "with user-provided volumes and volume mounts",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
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
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Len(t, pod.Spec.InitContainers[0].VolumeMounts, 3)
				assert.Len(t, pod.Spec.Volumes, 1)
				assert.Len(t, GetKibanaContainer(pod.Spec).VolumeMounts, 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPodTemplateSpec(context.Background(), k8s.NewFakeClient(), tt.kb, tt.keystore, []commonvolume.VolumeLike{})
			assert.NoError(t, err)
			tt.assertions(got)
		})
	}
}

func Test_getDefaultContainerPorts(t *testing.T) {
	tt := []struct {
		name string
		kb   kbv1.Kibana
		want []corev1.ContainerPort
	}{
		{
			name: "https",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					Version: "7.5.2",
				},
			},
			want: []corev1.ContainerPort{
				{Name: "https", HostPort: 0, ContainerPort: int32(network.HTTPPort), Protocol: "TCP", HostIP: ""},
			},
		},
		{
			name: "http",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					HTTP: commonv1.HTTPConfig{
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								Disabled: true,
							},
						},
					},
				},
			},
			want: []corev1.ContainerPort{
				{Name: "http", HostPort: 0, ContainerPort: int32(network.HTTPPort), Protocol: "TCP", HostIP: ""},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, getDefaultContainerPorts(tc.kb), tc.want)
		})
	}
}
