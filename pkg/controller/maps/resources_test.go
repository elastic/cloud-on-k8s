// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func mapsContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == emsv1alpha1.MapsContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func buildMapsPodTemplate(t *testing.T, ems emsv1alpha1.ElasticMapsServer) corev1.PodTemplateSpec {
	t.Helper()
	got, err := newPodSpec(ems, "test-hash", metadata.Metadata{}, false)
	require.NoError(t, err)
	return got
}

func TestMapsResources(t *testing.T) {
	base := emsv1alpha1.ElasticMapsServer{
		ObjectMeta: metav1.ObjectMeta{Name: "ems-test", Namespace: "default"},
	}

	for _, tt := range []struct {
		name   string
		ems    emsv1alpha1.ElasticMapsServer
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field is unset",
			ems: func() emsv1alpha1.ElasticMapsServer {
				e := base
				e.Spec = emsv1alpha1.MapsSpec{Version: "8.17.0"}
				return e
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory",
			ems: func() emsv1alpha1.ElasticMapsServer {
				e := base
				e.Spec = emsv1alpha1.MapsSpec{
					Version: "8.17.0",
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    ptr.To(resource.MustParse("500m")),
							Memory: ptr.To(resource.MustParse("1Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("1Gi")),
						},
					},
				}
				return e
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("500m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("1Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("1Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			ems: func() emsv1alpha1.ElasticMapsServer {
				e := base
				e.Spec = emsv1alpha1.MapsSpec{
					Version: "8.17.0",
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: emsv1alpha1.MapsContainerName,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("512Mi"),
											corev1.ResourceCPU:    resource.MustParse("250m"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("512Mi"),
										},
									},
								},
							},
						},
					},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("2Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("2Gi")),
						},
					},
				}
				return e
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("2Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("2Gi"), got.Limits[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("250m"), got.Requests[corev1.ResourceCPU])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildMapsPodTemplate(t, tt.ems)
			res, ok := mapsContainerResources(pod)
			require.True(t, ok, "maps container not found")
			tt.assert(t, res)
		})
	}
}
