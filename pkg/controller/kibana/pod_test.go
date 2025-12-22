// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"slices"
	"testing"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
				assert.Len(t, pod.Spec.Volumes, 2)
				kibanaContainer := GetKibanaContainer(pod.Spec)
				require.NotNil(t, kibanaContainer)
				assert.Equal(t, 2, len(kibanaContainer.VolumeMounts))
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
				assert.Len(t, pod.Spec.Volumes, 3)
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
			name:     "with user-provided labels, and 7.4.x shouldn't have security contexts set",
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
				assert.Nil(t, pod.Spec.SecurityContext)
				assert.Nil(t, GetKibanaContainer(pod.Spec).SecurityContext)
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
			name: "with user-provided volumes and 8.x should have volume mounts including /tmp and plugins volumes and security contexts",
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
				assert.Len(t, pod.Spec.InitContainers[0].VolumeMounts, 7)
				assert.Len(t, pod.Spec.Volumes, 5)
				assert.Len(t, GetKibanaContainer(pod.Spec).VolumeMounts, 5)
				assert.Equal(t, GetKibanaContainer(pod.Spec).SecurityContext, &defaultSecurityContext)
			},
		},
		{
			name: "with user-provided basePath in spec config",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server": map[string]interface{}{
							"basePath":        "/monitoring/kibana",
							"rewriteBasePath": true,
						},
					},
				},
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				kbContainer := GetKibanaContainer(pod.Spec)
				assert.Equal(t, kbContainer.ReadinessProbe.ProbeHandler.HTTPGet.Path, "/monitoring/kibana/login")
			},
		},
		{
			name: "with user-provided basePath in spec config flattened",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server.basePath":        "/monitoring/kibana",
						"server.rewriteBasePath": true,
					},
				},
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				kbContainer := GetKibanaContainer(pod.Spec)
				assert.Equal(t, kbContainer.ReadinessProbe.ProbeHandler.HTTPGet.Path, "/monitoring/kibana/login")
			},
		},
		{
			name: "with user-provided basePath in spec pod template",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
								Env: []corev1.EnvVar{
									{
										Name:  "SERVER_BASEPATH",
										Value: "/monitoring/kibana",
									},
									{
										Name:  "SERVER_REWRITEBASEPATH",
										Value: "true",
									},
								},
							},
						},
					},
				},
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				kbContainer := GetKibanaContainer(pod.Spec)
				assert.Equal(t, kbContainer.ReadinessProbe.ProbeHandler.HTTPGet.Path, "/monitoring/kibana/login")
			},
		},
		{
			name: "with user-provided basePath in spec config but rewriteBasePath not set",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server": map[string]interface{}{
							"basePath": "/monitoring/kibana",
						},
					},
				},
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				kbContainer := GetKibanaContainer(pod.Spec)
				assert.Equal(t, kbContainer.ReadinessProbe.ProbeHandler.HTTPGet.Path, "/login")
			},
		},
		{
			name: "with user-provided basePath in spec pod template but rewriteBasePath not set",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
								Env: []corev1.EnvVar{
									{
										Name:  "SERVER_BASEPATH",
										Value: "/monitoring/kibana",
									},
								},
							},
						},
					},
				},
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				kbContainer := GetKibanaContainer(pod.Spec)
				assert.Equal(t, kbContainer.ReadinessProbe.ProbeHandler.HTTPGet.Path, "/login")
			},
		},
		{
			name: "with user-provided basePath in spec pod template and spec config, env var in pod template should take precedence",
			kb: kbv1.Kibana{Spec: kbv1.KibanaSpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server": map[string]interface{}{
							"basePath":        "/monitoring/kibana/spec",
							"rewriteBasePath": true,
						},
					},
				},
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
								Env: []corev1.EnvVar{
									{
										Name:  "SERVER_BASEPATH",
										Value: "/monitoring/kibana",
									},
									{
										Name:  "SERVER_REWRITEBASEPATH",
										Value: "true",
									},
								},
							},
						},
					},
				},
				Version: "8.12.0",
			}},
			assertions: func(pod corev1.PodTemplateSpec) {
				kbContainer := GetKibanaContainer(pod.Spec)
				assert.Equal(t, kbContainer.ReadinessProbe.ProbeHandler.HTTPGet.Path, "/monitoring/kibana/login")
			},
		},
		{
			name: "with EPR association and user-provided NODE_EXTRA_CA_CERTS should pass env var to init container",
			kb: func() kbv1.Kibana {
				kb := kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kibana",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"association.k8s.elastic.co/epr-conf": `{"authSecretName":"-","authSecretKey":"","caCertProvided":true,"caSecretName":"test-ca","url":"https://test-epr:8080"}`,
						},
					},
					Spec: kbv1.KibanaSpec{
						Version: "7.1.0",
						PackageRegistryRef: commonv1.ObjectSelector{
							Name: "test-epr",
						},
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: kbv1.KibanaContainerName,
										Env: []corev1.EnvVar{
											{
												Name:  nodeExtraCACerts,
												Value: "/custom/user/ca-bundle.crt",
											},
										},
									},
								},
							},
						},
					},
				}
				return kb
			}(),
			assertions: func(pod corev1.PodTemplateSpec) {
				// Main container should have NODE_EXTRA_CA_CERTS pointing to combined bundle
				container := GetKibanaContainer(pod.Spec)
				assertEnvValue(t, container, nodeExtraCACerts, "/usr/share/kibana/config/combined-ca-bundle.crt", "Main container should have NODE_EXTRA_CA_CERTS pointing to combined bundle")

				// Init container should also have NODE_EXTRA_CA_CERTS
				assert.Len(t, pod.Spec.InitContainers, 1)
				initContainer := pod.Spec.InitContainers[0]
				assertEnvValue(t, &initContainer, nodeExtraCACerts, "/custom/user/ca-bundle.crt", "Init container should have NODE_EXTRA_CA_CERTS for EPR CA appending")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp, err := GetKibanaBasePath(tt.kb)
			require.NoError(t, err)
			md := metadata.Propagate(&tt.kb, metadata.Metadata{Labels: tt.kb.GetIdentityLabels()})
			got, err := NewPodTemplateSpec(context.Background(), k8s.NewFakeClient(), tt.kb, tt.keystore, []commonvolume.VolumeLike{}, bp, true, md)
			assert.NoError(t, err)
			tt.assertions(got)
		})
	}
}

func TestWithEPRCertsVolume(t *testing.T) {
	tests := []struct {
		name       string
		kb         kbv1.Kibana
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "without EPR association",
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kibana",
					Namespace: "test-ns",
				},
				Spec: kbv1.KibanaSpec{
					Version: "7.1.0",
				},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				// Should not have NODE_EXTRA_CA_CERTS environment variable
				container := GetKibanaContainer(pod.Spec)
				assertEnvNotSet(t, container, nodeExtraCACerts, "Should not have NODE_EXTRA_CA_CERTS environment variable")
			},
		},
		{
			name: "with EPR association but no CA configured",
			kb: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kibana",
					Namespace: "test-ns",
				},
				Spec: kbv1.KibanaSpec{
					Version: "7.1.0",
					PackageRegistryRef: commonv1.ObjectSelector{
						Name: "test-epr",
					},
				},
			},
			assertions: func(pod corev1.PodTemplateSpec) {
				// Should not have NODE_EXTRA_CA_CERTS since CA is not configured
				container := GetKibanaContainer(pod.Spec)
				assertEnvNotSet(t, container, nodeExtraCACerts, "Should not have NODE_EXTRA_CA_CERTS environment variable")
			},
		},
		{
			name: "with EPR association and CA configured",
			kb: func() kbv1.Kibana {
				kb := kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kibana",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"association.k8s.elastic.co/epr-conf": `{"authSecretName":"-","authSecretKey":"","caCertProvided":true,"caSecretName":"test-ca","url":"https://test-epr:8080"}`,
						},
					},
					Spec: kbv1.KibanaSpec{
						Version: "7.1.0",
						PackageRegistryRef: commonv1.ObjectSelector{
							Name: "test-epr",
						},
					},
				}
				return kb
			}(),
			assertions: func(pod corev1.PodTemplateSpec) {
				// Should have NODE_EXTRA_CA_CERTS environment variable
				container := GetKibanaContainer(pod.Spec)
				assertEnvValue(t, container, nodeExtraCACerts, eprCertsVolumeMountPath+"/ca.crt", "NODE_EXTRA_CA_CERTS environment variable should be set")
			},
		},
		{
			name: "respects user-provided NODE_EXTRA_CA_CERTS",
			kb: func() kbv1.Kibana {
				kb := kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kibana",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"association.k8s.elastic.co/epr-conf": `{"authSecretName":"-","authSecretKey":"","caCertProvided":true,"caSecretName":"test-ca","url":"https://test-epr:8080"}`,
						},
					},
					Spec: kbv1.KibanaSpec{
						Version: "7.1.0",
						PackageRegistryRef: commonv1.ObjectSelector{
							Name: "test-epr",
						},
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: kbv1.KibanaContainerName,
										Env: []corev1.EnvVar{
											{
												Name:  nodeExtraCACerts,
												Value: "/custom/ca/bundle.crt",
											},
										},
									},
								},
							},
						},
					},
				}
				return kb
			}(),
			assertions: func(pod corev1.PodTemplateSpec) {
				// Should update NODE_EXTRA_CA_CERTS to combined bundle path when user provides their own
				container := GetKibanaContainer(pod.Spec)
				assertEnvValue(t, container, nodeExtraCACerts, "/usr/share/kibana/config/combined-ca-bundle.crt", "NODE_EXTRA_CA_CERTS environment variable should be set")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp, err := GetKibanaBasePath(tt.kb)
			require.NoError(t, err)
			md := metadata.Propagate(&tt.kb, metadata.Metadata{Labels: tt.kb.GetIdentityLabels()})
			got, err := NewPodTemplateSpec(context.Background(), k8s.NewFakeClient(), tt.kb, nil, []commonvolume.VolumeLike{}, bp, true, md)
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

func assertEnvNotSet(t *testing.T, container *corev1.Container, envName string, message string) {
	t.Helper()
	contains := slices.ContainsFunc(container.Env, func(env corev1.EnvVar) bool {
		return env.Name == envName
	})
	assert.False(t, contains, message)
}

func assertEnvValue(t *testing.T, container *corev1.Container, envName, expectedValue, message string) {
	t.Helper()
	var found bool
	for _, env := range container.Env {
		if env.Name == envName {
			assert.Equal(t, expectedValue, env.Value, message)
			found = true
			break
		}
	}
	assert.True(t, found, message)
}
