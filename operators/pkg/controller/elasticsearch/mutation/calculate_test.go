// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var defaultCPULimit = "800m"
var defaultImage = "image"
var defaultPodSpecCtxV2 = ESPodSpecContext(defaultImage, "1000m")

func ESPodWithConfig(image string, cpuLimit string) pod.PodWithConfig {
	return pod.PodWithConfig{
		Pod:    corev1.Pod{Spec: ESPodSpecContext(image, cpuLimit).PodSpec},
		Config: settings.FlatConfig{},
	}
}

func ESPodSpecContext(image string, cpuLimit string) pod.PodSpecContext {
	return pod.PodSpecContext{
		PodSpec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elasticsearch",
				Ports:           version.DefaultContainerPorts,
				// TODO: Hardcoded resource limits and requests
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(cpuLimit),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				ReadinessProbe: &corev1.Probe{
					FailureThreshold:    3,
					InitialDelaySeconds: 10,
					PeriodSeconds:       10,
					SuccessThreshold:    3,
					TimeoutSeconds:      5,
					Handler: corev1.Handler{
						Exec: &corev1.ExecAction{
							Command: []string{
								"sh",
								"-c",
								"script here",
							},
						},
					},
				},
			}},
		},
	}
}

func TestCalculateChanges(t *testing.T) {
	type args struct {
		expected []pod.PodSpecContext
		state    reconcile.ResourcesState
	}
	tests := []struct {
		name string
		args args
		want Changes
	}{
		{
			name: "Wait for 2 pods to be terminated, create 1",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx},
				state:    reconcile.ResourcesState{DeletingPods: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
			},
			want: Changes{
				ToKeep:   pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig},
				ToCreate: []PodToCreate{{PodSpecCtx: defaultPodSpecCtx}},
			},
		},
		{
			name: "Do not wait for 2 pods to be terminated, create 3",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtxV2, defaultPodSpecCtxV2, defaultPodSpecCtxV2},
				state:    reconcile.ResourcesState{DeletingPods: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
			},
			want: Changes{
				ToKeep:   pod.PodsWithConfig{},
				ToDelete: pod.PodsWithConfig{},
				ToCreate: []PodToCreate{{PodSpecCtx: defaultPodSpecCtxV2}, {PodSpecCtx: defaultPodSpecCtxV2}, {PodSpecCtx: defaultPodSpecCtxV2}},
			},
		},
		{
			name: "no changes",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx},
				state:    reconcile.ResourcesState{CurrentPods: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
			},
			want: Changes{ToKeep: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
		},
		{
			name: "2 new pods",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx},
				state:    reconcile.ResourcesState{CurrentPods: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
			},
			want: Changes{
				ToKeep:   pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig},
				ToCreate: []PodToCreate{{PodSpecCtx: defaultPodSpecCtx}, {PodSpecCtx: defaultPodSpecCtx}},
			},
		},
		{
			name: "2 less pods",
			args: args{
				expected: []pod.PodSpecContext{},
				state:    reconcile.ResourcesState{CurrentPods: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
			},
			want: Changes{ToDelete: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
		},
		{
			name: "1 pod replaced",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, ESPodSpecContext("another-image", defaultCPULimit)},
				state:    reconcile.ResourcesState{CurrentPods: pod.PodsWithConfig{defaultPodWithConfig, defaultPodWithConfig}},
			},
			want: Changes{
				ToKeep:   pod.PodsWithConfig{defaultPodWithConfig},
				ToDelete: pod.PodsWithConfig{defaultPodWithConfig},
				ToCreate: []PodToCreate{{PodSpecCtx: ESPodSpecContext("another-image", defaultCPULimit)}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reuseOptions := ReuseOptions{
				ReusePVCs: false,
				ReusePods: false,
			}
			got, err := CalculateChanges(
				tt.args.expected,
				tt.args.state,
				func(ctx pod.PodSpecContext) (corev1.Pod, error) {
					return corev1.Pod{}, nil // TODO: fix
				},
				reuseOptions,
			)
			assert.NoError(t, err)
			assert.Equal(t, len(tt.want.ToKeep), len(got.ToKeep))
			assert.Equal(t, len(tt.want.ToCreate), len(got.ToCreate))
			assert.Equal(t, len(tt.want.ToDelete), len(got.ToDelete))
		})
	}
}
