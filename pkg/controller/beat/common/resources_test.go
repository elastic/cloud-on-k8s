// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func beatContainerResources(typ string, pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == typ {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func buildBeatPodTemplate(t *testing.T, beat beatv1beta1.Beat) corev1.PodTemplateSpec {
	t.Helper()
	params := DriverParams{
		Context: context.Background(),
		Client:  k8s.NewFakeClient(),
		Watches: watches.NewDynamicWatches(),
		Beat:    beat,
	}
	configHash := fnv.New32a()
	pod, err := buildPodTemplate(params, container.FilebeatImage, configHash, metadata.Metadata{})
	require.NoError(t, err)
	return pod
}

func newMinimalBeat(spec beatv1beta1.BeatSpec) beatv1beta1.Beat {
	return beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{Name: "beat-test", Namespace: "default"},
		Spec:       spec,
	}
}

func TestBeatResources(t *testing.T) {
	const beatType = "filebeat"

	for _, tt := range []struct {
		name   string
		beat   beatv1beta1.Beat
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field is unset",
			beat: newMinimalBeat(beatv1beta1.BeatSpec{
				Type:      beatType,
				Version:   "8.17.0",
				DaemonSet: &beatv1beta1.DaemonSetSpec{},
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, defaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory",
			beat: newMinimalBeat(beatv1beta1.BeatSpec{
				Type:      beatType,
				Version:   "8.17.0",
				DaemonSet: &beatv1beta1.DaemonSetSpec{},
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("100m")),
						Memory: ptr.To(resource.MustParse("256Mi")),
					},
					Limits: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("256Mi")),
					},
				},
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("100m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("256Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("256Mi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			beat: newMinimalBeat(beatv1beta1.BeatSpec{
				Type:    beatType,
				Version: "8.17.0",
				DaemonSet: &beatv1beta1.DaemonSetSpec{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: beatType,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("128Mi"),
											corev1.ResourceCPU:    resource.MustParse("50m"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
								},
							},
						},
					},
				},
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("512Mi")),
					},
					Limits: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("512Mi")),
					},
				},
			}),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("512Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("512Mi"), got.Limits[corev1.ResourceMemory])
				// cpu preserved from podTemplate
				require.Equal(t, resource.MustParse("50m"), got.Requests[corev1.ResourceCPU])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildBeatPodTemplate(t, tt.beat)
			res, ok := beatContainerResources(beatType, pod)
			require.True(t, ok, "beat container %q not found", beatType)
			tt.assert(t, res)
		})
	}
}
