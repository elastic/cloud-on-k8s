// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
)

func entContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == entv1.EnterpriseSearchContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func buildEntPodTemplate(t *testing.T, ent entv1.EnterpriseSearch) corev1.PodTemplateSpec {
	t.Helper()
	got, err := newPodSpec(ent, "test-hash")
	require.NoError(t, err)
	return got
}

func TestEnterpriseSearchResources(t *testing.T) {
	base := entv1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{Name: "ent-test", Namespace: "default"},
	}

	for _, tt := range []struct {
		name   string
		ent    entv1.EnterpriseSearch
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field is unset",
			ent: func() entv1.EnterpriseSearch {
				e := base
				e.Spec = entv1.EnterpriseSearchSpec{Version: "8.17.0"}
				return e
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory",
			ent: func() entv1.EnterpriseSearch {
				e := base
				e.Spec = entv1.EnterpriseSearchSpec{
					Version: "8.17.0",
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    ptr.To(resource.MustParse("2")),
							Memory: ptr.To(resource.MustParse("4Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("4Gi")),
						},
					},
				}
				return e
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("2"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("4Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("4Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			ent: func() entv1.EnterpriseSearch {
				e := base
				e.Spec = entv1.EnterpriseSearchSpec{
					Version: "8.17.0",
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: entv1.EnterpriseSearchContainerName,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("1Gi"),
											corev1.ResourceCPU:    resource.MustParse("500m"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("1Gi"),
										},
									},
								},
							},
						},
					},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("3Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("3Gi")),
						},
					},
				}
				return e
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				// memory overridden
				require.Equal(t, resource.MustParse("3Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("3Gi"), got.Limits[corev1.ResourceMemory])
				// cpu preserved from podTemplate
				require.Equal(t, resource.MustParse("500m"), got.Requests[corev1.ResourceCPU])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildEntPodTemplate(t, tt.ent)
			res, ok := entContainerResources(pod)
			require.True(t, ok, "enterprise-search container not found")
			tt.assert(t, res)
		})
	}
}
