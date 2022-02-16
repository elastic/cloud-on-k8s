// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	testAgentNsn = metav1.ObjectMeta{
		Name:      "fake-apm",
		Namespace: "default",
	}
	// while the associations are optional the HTTP certs Secret has to exist to calculate the config hash and successfully build the pod spec
	testHTTPCertsInternalSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-apm-apm-http-certs-internal",
			Namespace: "default",
		},
	}
)

func TestNewPodSpec(t *testing.T) {
	configSecretVol := volume.NewSecretVolumeWithMountPath(
		"config-secret",
		"config",
		"/usr/share/apm-server/config/config-secret",
	)
	httpCertsSecretVol := volume.NewSecretVolumeWithMountPath(
		"fake-apm-apm-http-certs-internal",
		"elastic-internal-http-certificates",
		"/mnt/elastic-internal/http-certs",
	)
	varFalse := false
	probe := readinessProbe(true)
	tests := []struct {
		name string
		as   apmv1.ApmServer
		p    PodSpecParams
		want corev1.PodTemplateSpec
	}{
		{
			name: "create default pod spec",
			as: apmv1.ApmServer{
				TypeMeta: metav1.TypeMeta{
					Kind: apmv1.Kind,
				},
				ObjectMeta: testAgentNsn,
			},
			p: PodSpecParams{
				Version: "7.0.1",
				ConfigSecret: corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "config-secret",
					},
				},
				TokenSecret: corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "token-secret",
					},
				},
			},
			want: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"apm.k8s.elastic.co/name":    "fake-apm",
						"apm.k8s.elastic.co/version": "7.0.1",
						"common.k8s.elastic.co/type": "apm-server",
					},
					Annotations: map[string]string{
						"apm.k8s.elastic.co/config-hash": "2166136261",
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						configSecretVol.Volume(), configVolume.Volume(), httpCertsSecretVol.Volume(),
					},
					AutomountServiceAccountToken: &varFalse,
					Containers: []corev1.Container{
						{
							Name:  apmv1.ApmServerContainerName,
							Image: container.ImageRepository(container.APMServerImage, "7.0.1"),
							Env: []corev1.EnvVar{
								{
									Name: settings.EnvPodIP,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
									},
								},
								{
									Name: settings.EnvPodName,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
									},
								},
								{
									Name: settings.EnvNodeName,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"},
									},
								},
								{Name: settings.EnvNamespace,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"},
									}},
								{
									Name: "SECRET_TOKEN",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: "token-secret"},
											Key:                  SecretTokenKey,
										},
									},
								},
							},
							ReadinessProbe: &probe,
							Ports:          []corev1.ContainerPort{{Name: "https", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP}},
							Command:        command,
							VolumeMounts: []corev1.VolumeMount{
								configSecretVol.VolumeMount(), configVolume.VolumeMount(), httpCertsSecretVol.VolumeMount(),
							},
							Resources: DefaultResources,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newPodSpec(k8s.NewFakeClient(&testHTTPCertsInternalSecret), &tt.as, tt.p)
			assert.NoError(t, err)
			diff := deep.Equal(tt.want, got)
			assert.Empty(t, diff)
		})
	}
}

func Test_getDefaultContainerPorts(t *testing.T) {
	tt := []struct {
		name string
		as   apmv1.ApmServer
		want []corev1.ContainerPort
	}{
		{
			name: "https",
			as: apmv1.ApmServer{
				Spec: apmv1.ApmServerSpec{
					Version: "7.5.2",
				},
			},
			want: []corev1.ContainerPort{
				{Name: "https", HostPort: 0, ContainerPort: int32(HTTPPort), Protocol: "TCP", HostIP: ""},
			},
		},
		{
			name: "http",
			as: apmv1.ApmServer{
				Spec: apmv1.ApmServerSpec{
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
				{Name: "http", HostPort: 0, ContainerPort: int32(HTTPPort), Protocol: "TCP", HostIP: ""},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, getDefaultContainerPorts(tc.as), tc.want)
		})
	}
}

func Test_newPodSpec_withInitContainers(t *testing.T) {
	tests := []struct {
		name       string
		as         apmv1.ApmServer
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "user-provided init containers should inherit from the default main container image",
			as: apmv1.ApmServer{
				ObjectMeta: testAgentNsn,
				Spec: apmv1.ApmServerSpec{
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
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
				assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, pod.Spec.InitContainers[0].VolumeMounts)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := PodSpecParams{
				Version:         tt.as.Spec.Version,
				CustomImageName: tt.as.Spec.Image,
				PodTemplate:     tt.as.Spec.PodTemplate,
			}
			got, err := newPodSpec(k8s.NewFakeClient(&testHTTPCertsInternalSecret), &tt.as, params)
			assert.NoError(t, err)
			tt.assertions(got)
		})
	}
}
