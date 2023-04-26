// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestNewPodTemplateSpec(t *testing.T) {
	tests := []struct {
		name       string
		logstash   logstashv1alpha1.Logstash
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "defaults",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.6.1",
				},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, false, *pod.Spec.AutomountServiceAccountToken)
				assert.Len(t, pod.Spec.Containers, 1)
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Len(t, pod.Spec.Volumes, 3)
				assert.NotEmpty(t, pod.Annotations[ConfigHashAnnotationName])
				logstashContainer := GetLogstashContainer(pod.Spec)
				require.NotNil(t, logstashContainer)
				assert.Equal(t, 3, len(logstashContainer.VolumeMounts))
				assert.Equal(t, container.ImageRepository(container.LogstashImage, "8.6.1"), logstashContainer.Image)
				assert.NotNil(t, logstashContainer.ReadinessProbe)
				assert.NotEmpty(t, logstashContainer.Ports)
			},
		},
		{
			name: "with custom image",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
				Image:   "my-custom-image:1.0.0",
				Version: "8.6.1",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, "my-custom-image:1.0.0", GetLogstashContainer(pod.Spec).Image)
			},
		},
		{
			name: "with default resources",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
				Version: "8.6.1",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, DefaultResources, GetLogstashContainer(pod.Spec).Resources)
			},
		},
		{
			name: "with user-provided resources",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
				Version: "8.6.1",
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "logstash",
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
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
				}, GetLogstashContainer(pod.Spec).Resources)
			},
		},
		{
			name: "with user-provided init containers",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
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
				assert.Len(t, pod.Spec.InitContainers, 2)
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
			},
		},
		{
			name: "with user-provided labels",
			logstash: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name: "logstash-name",
				},
				Spec: logstashv1alpha1.LogstashSpec{
					PodTemplate: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"label1":      "value1",
								"label2":      "value2",
								NameLabelName: "overridden-logstash-name",
							},
						},
					},
					Version: "8.6.1",
				}},
			assertions: func(pod corev1.PodTemplateSpec) {
				labels := (&logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Name: "logstash-name"}}).GetIdentityLabels()
				labels[VersionLabelName] = "8.6.1"
				labels["label1"] = "value1"
				labels["label2"] = "value2"
				labels[NameLabelName] = "overridden-logstash-name"
				assert.Equal(t, labels, pod.Labels)
			},
		},
		{
			name: "with user-provided ENV variable",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "logstash",
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
				assert.Len(t, GetLogstashContainer(pod.Spec).Env, 1)
			},
		},
		{
			name: "with multiple services, readiness probe hits the correct port",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.6.1",
					Services: []logstashv1alpha1.LogstashService{{
						Name: LogstashAPIServiceName,
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Name: "api", Protocol: "TCP", Port: 9200},
								},
							},
						}}, {
						Name: "notapi",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Name: "notapi", Protocol: "TCP", Port: 9600},
								},
							},
						}},
					},
				},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, 9200, GetLogstashContainer(pod.Spec).ReadinessProbe.HTTPGet.Port.IntValue())
			},
		},
		{
			name: "with api service customized, readiness probe hits the correct port",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.6.1",
					Services: []logstashv1alpha1.LogstashService{
						{
							Name: LogstashAPIServiceName,
							Service: commonv1.ServiceTemplate{
								Spec: corev1.ServiceSpec{
									Ports: []corev1.ServicePort{
										{Name: "api", Protocol: "TCP", Port: 9200},
									},
								},
							}},
					},
				}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, 9200, GetLogstashContainer(pod.Spec).ReadinessProbe.HTTPGet.Port.IntValue())
			},
		},
		{
			name: "with default service, readiness probe hits the correct port",
			logstash: logstashv1alpha1.Logstash{
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.6.1",
				}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, 9600, GetLogstashContainer(pod.Spec).ReadinessProbe.HTTPGet.Port.IntValue())
			},
		},

		{
			name: "with custom annotation",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
				Image:   "my-custom-image:1.0.0",
				Version: "8.6.1",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Equal(t, "my-custom-image:1.0.0", GetLogstashContainer(pod.Spec).Image)
			},
		},
		{
			name: "with user-provided volumes and volume mounts",
			logstash: logstashv1alpha1.Logstash{Spec: logstashv1alpha1.LogstashSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "logstash",
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
				assert.Len(t, pod.Spec.Volumes, 4)
				assert.Len(t, GetLogstashContainer(pod.Spec).VolumeMounts, 4)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := Params{
				Context:  context.Background(),
				Client:   k8s.NewFakeClient(),
				Logstash: tt.logstash,
			}
			configHash := fnv.New32a()
			got, err := buildPodTemplate(params, configHash)

			require.NoError(t, err)
			tt.assertions(got)
		})
	}
}

// GetLogstashContainer returns the Logstash container from the given podSpec.
func GetLogstashContainer(podSpec corev1.PodSpec) *corev1.Container {
	return pod.ContainerByName(podSpec, logstashv1alpha1.LogstashContainerName)
}
