// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func eprContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == eprv1alpha1.EPRContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func newMinimalEPR(spec eprv1alpha1.PackageRegistrySpec) eprv1alpha1.PackageRegistry {
	return eprv1alpha1.PackageRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "epr-test", Namespace: "default"},
		Spec:       spec,
	}
}

func TestPackageRegistryResources(t *testing.T) {
	for _, tt := range []struct {
		name   string
		epr    eprv1alpha1.PackageRegistry
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field and podTemplate are both unset",
			epr: newMinimalEPR(eprv1alpha1.PackageRegistrySpec{
				Version: "8.15.0",
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory on EPR container",
			epr: newMinimalEPR(eprv1alpha1.PackageRegistrySpec{
				Version: "8.15.0",
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("250m")),
						Memory: ptr.To(resource.MustParse("512Mi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("500m")),
						Memory: ptr.To(resource.MustParse("2Gi")),
					},
				},
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("250m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("512Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("500m"), got.Limits[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("2Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			epr: newMinimalEPR(eprv1alpha1.PackageRegistrySpec{
				Version: "8.15.0",
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: eprv1alpha1.EPRContainerName,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
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
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("2Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("2Gi"), got.Limits[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("100m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("200m"), got.Limits[corev1.ResourceCPU])
			},
		},
		{
			name: "podTemplate-only resources work without resources field",
			epr: newMinimalEPR(eprv1alpha1.PackageRegistrySpec{
				Version: "8.15.0",
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: eprv1alpha1.EPRContainerName,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("3Gi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("3Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("3Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod, err := newPodSpec(tt.epr, "test-hash", metadata.Metadata{}, false)
			require.NoError(t, err)
			res, ok := eprContainerResources(pod)
			require.True(t, ok, "EPR container not found")
			tt.assert(t, res)
		})
	}
}
