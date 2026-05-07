// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func kibanaContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == kbv1.KibanaContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func newMinimalKibana(spec kbv1.KibanaSpec) kbv1.Kibana {
	return kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Name: "kibana-test", Namespace: "default"},
		Spec:       spec,
	}
}

func buildKibanaPodTemplate(t *testing.T, kb kbv1.Kibana) corev1.PodTemplateSpec {
	t.Helper()
	got, err := NewPodTemplateSpec(
		context.Background(),
		k8s.NewFakeClient(),
		kb,
		nil,
		[]commonvolume.VolumeLike{},
		"",
		false,
		metadata.Metadata{},
	)
	require.NoError(t, err)
	return got
}

func TestKibanaResources(t *testing.T) {
	for _, tt := range []struct {
		name   string
		kb     kbv1.Kibana
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field and podTemplate are both unset",
			kb: newMinimalKibana(kbv1.KibanaSpec{
				Version: "8.17.0",
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory on kibana container",
			kb: newMinimalKibana(kbv1.KibanaSpec{
				Version: "8.17.0",
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("500m")),
						Memory: ptr.To(resource.MustParse("1Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("1")),
						Memory: ptr.To(resource.MustParse("1Gi")),
					},
				},
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("500m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("1Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("1"), got.Limits[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("1Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			kb: newMinimalKibana(kbv1.KibanaSpec{
				Version: "8.17.0",
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
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
				// memory overridden by resources field
				require.Equal(t, resource.MustParse("2Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("2Gi"), got.Limits[corev1.ResourceMemory])
				// cpu preserved from podTemplate
				require.Equal(t, resource.MustParse("100m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("200m"), got.Limits[corev1.ResourceCPU])
			},
		},
		{
			name: "podTemplate-only resources work without resources field",
			kb: newMinimalKibana(kbv1.KibanaSpec{
				Version: "8.17.0",
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
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
			pod := buildKibanaPodTemplate(t, tt.kb)
			res, ok := kibanaContainerResources(pod)
			require.True(t, ok, "kibana container not found")
			tt.assert(t, res)
		})
	}
}
