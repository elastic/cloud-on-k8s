// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func apmContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == apmv1.ApmServerContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func minimalApmParams(as apmv1.ApmServer) PodSpecParams {
	return PodSpecParams{
		Version:      as.Spec.Version,
		PodTemplate:  as.Spec.PodTemplate,
		ConfigSecret: corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"}},
		TokenSecret:  corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tok", Namespace: "default"}},
	}
}

func buildApmPodTemplate(t *testing.T, as apmv1.ApmServer) corev1.PodTemplateSpec {
	t.Helper()
	// newPodSpec calls buildConfigHash which fetches the HTTP certs secret; provide a minimal one.
	httpCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      as.Name + "-apm-http-certs-internal",
			Namespace: as.Namespace,
		},
	}
	got, err := newPodSpec(k8s.NewFakeClient(&httpCertsSecret), &as, minimalApmParams(as), metadata.Metadata{}, false)
	require.NoError(t, err)
	return got
}

func TestApmResources(t *testing.T) {
	base := apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{Name: "apm-test", Namespace: "default"},
	}

	for _, tt := range []struct {
		name   string
		as     apmv1.ApmServer
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field is unset",
			as: func() apmv1.ApmServer {
				a := base
				a.Spec = apmv1.ApmServerSpec{Version: "8.17.0"}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory",
			as: func() apmv1.ApmServer {
				a := base
				a.Spec = apmv1.ApmServerSpec{
					Version: "8.17.0",
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    ptr.To(resource.MustParse("300m")),
							Memory: ptr.To(resource.MustParse("512Mi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("512Mi")),
						},
					},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("300m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("512Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("512Mi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			as: func() apmv1.ApmServer {
				a := base
				a.Spec = apmv1.ApmServerSpec{
					Version: "8.17.0",
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: apmv1.ApmServerContainerName,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("256Mi"),
											corev1.ResourceCPU:    resource.MustParse("50m"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
								},
							},
						},
					},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("1Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("1Gi")),
						},
					},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				// memory overridden
				require.Equal(t, resource.MustParse("1Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("1Gi"), got.Limits[corev1.ResourceMemory])
				// cpu preserved from podTemplate
				require.Equal(t, resource.MustParse("50m"), got.Requests[corev1.ResourceCPU])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildApmPodTemplate(t, tt.as)
			res, ok := apmContainerResources(pod)
			require.True(t, ok, "apm-server container not found")
			tt.assert(t, res)
		})
	}
}
