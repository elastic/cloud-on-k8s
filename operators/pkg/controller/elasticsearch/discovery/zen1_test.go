// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package discovery

import (
	"testing"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
}

// newPod6 creates a new named potentially labeled as master
func newPod6(name string, master bool) pod.PodWithConfig {
	return newPodWithVersion(name, master, "6.8.0")
}

func podConfig(podName, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      settings.ConfigSecretName(podName),
		},
		Data: map[string][]byte{
			settings.ConfigFileName: []byte("a: b\nc: d\n"),
		},
	}
}

func TestApplyZen1Limitations(t *testing.T) {
	type args struct {
		c                        k8s.Client
		podsState                mutation.PodsState
		performableChanges       *mutation.PerformableChanges
		isElasticsearchReachable bool
	}

	pod1WithConfig := newPod6("one", true)
	pod2WithConfig := newPod6("two", true)
	pod3WithConfig := newPod6("three", true)
	pod4WithConfig := newPod6("four", true)
	pod5WithConfig := newPod6("five", true)

	tests := []struct {
		name       string
		args       args
		assertions func(t *testing.T, performableChanges *mutation.PerformableChanges)
		wantErr    bool
	}{
		{
			name: "from 1 to 3 masters should add just one",
			args: args{
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: mutation.PodsToCreate{
							mutation.PodToCreate{Pod: pod2WithConfig.Pod},
							mutation.PodToCreate{Pod: pod3WithConfig.Pod},
						},
					},
				},
				isElasticsearchReachable: true,
			},
			assertions: func(t *testing.T, performableChanges *mutation.PerformableChanges) {
				assert.Len(t, performableChanges.ToCreate, 1)
			},
		},
		{
			name: "from 3 to 5 masters should add two",
			args: args{
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
						pod2WithConfig.Pod.Name: pod2WithConfig.Pod,
						pod3WithConfig.Pod.Name: pod3WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: mutation.PodsToCreate{
							mutation.PodToCreate{
								Pod: pod4WithConfig.Pod,
							},
							mutation.PodToCreate{
								Pod: pod5WithConfig.Pod,
							},
						},
					},
				},
				isElasticsearchReachable: true,
			},
			assertions: func(t *testing.T, performableChanges *mutation.PerformableChanges) {
				assert.Len(t, performableChanges.ToCreate, 2)
			},
		},
		{
			name: "from 3 to 1 masters should delete 1",
			args: args{
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
						pod2WithConfig.Pod.Name: pod2WithConfig.Pod,
						pod3WithConfig.Pod.Name: pod3WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToDelete: pod.PodsWithConfig{
							pod2WithConfig,
							pod3WithConfig,
						},
					},
				},
				isElasticsearchReachable: true,
			},
			assertions: func(t *testing.T, performableChanges *mutation.PerformableChanges) {
				assert.Len(t, performableChanges.ToDelete, 1)
			},
		},
		{
			name: "from 5 to 3 masters should delete two",
			args: args{
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
						pod2WithConfig.Pod.Name: pod2WithConfig.Pod,
						pod3WithConfig.Pod.Name: pod3WithConfig.Pod,
						pod4WithConfig.Pod.Name: pod4WithConfig.Pod,
						pod5WithConfig.Pod.Name: pod5WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToDelete: pod.PodsWithConfig{
							pod4WithConfig,
							pod5WithConfig,
						},
					},
				},
				isElasticsearchReachable: true,
			},
			assertions: func(t *testing.T, performableChanges *mutation.PerformableChanges) {
				assert.Len(t, performableChanges.ToDelete, 2)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ApplyZen1Limitations(tt.args.c, tt.args.podsState, tt.args.performableChanges, tt.args.isElasticsearchReachable); (err != nil) != tt.wantErr {
				t.Errorf("ApplyZen1Limitations() error = %v, wantErr %v", err, tt.wantErr)
			}

			tt.assertions(t, tt.args.performableChanges)
		})
	}
}

func TestZen1UpdateMinimumMasterNodesConfig(t *testing.T) {
	type args struct {
		c                  k8s.Client
		es                 v1alpha1.Elasticsearch
		podsState          mutation.PodsState
		performableChanges *mutation.PerformableChanges
	}

	pod1WithConfig := newPod6("one", true)
	pod2WithConfig := newPod6("two", true)
	pod3WithConfig := newPod6("three", true)
	pod4WithConfig := newPod6("four", true)
	pod5WithConfig := newPod6("five", true)

	tests := []struct {
		name       string
		args       args
		assertions func(t *testing.T, c k8s.Client, performableChanges *mutation.PerformableChanges)
		wantErr    bool
	}{
		{
			name: "from 1 to 2 master pods",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(podConfig(pod1WithConfig.Pod.Name, pod1WithConfig.Pod.Namespace))),
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: mutation.PodsToCreate{
							mutation.PodToCreate{
								Pod: pod2WithConfig.Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: pod2WithConfig.Config,
								},
							},
						},
					},
				},
			},
			assertions: func(t *testing.T, c k8s.Client, performableChanges *mutation.PerformableChanges) {
				assertMinimumMasterNodes(t, 2, c, performableChanges, []corev1.Pod{pod1WithConfig.Pod})
			},
		},
		{
			name: "from 2 to 1 master pods",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(
					podConfig(pod1WithConfig.Pod.Name, pod1WithConfig.Pod.Namespace),
					podConfig(pod2WithConfig.Pod.Name, pod2WithConfig.Pod.Namespace),
				)),
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
						pod2WithConfig.Pod.Name: pod2WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToDelete: pod.PodsWithConfig{
							pod2WithConfig,
						},
					},
				},
			},
			assertions: func(t *testing.T, c k8s.Client, performableChanges *mutation.PerformableChanges) {
				assertMinimumMasterNodes(t, 1, c, performableChanges, []corev1.Pod{pod1WithConfig.Pod})
			},
		},
		{
			name: "from 5 to 3 master pods",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(
					podConfig(pod1WithConfig.Pod.Name, pod1WithConfig.Pod.Namespace),
					podConfig(pod2WithConfig.Pod.Name, pod2WithConfig.Pod.Namespace),
					podConfig(pod3WithConfig.Pod.Name, pod3WithConfig.Pod.Namespace),
					podConfig(pod4WithConfig.Pod.Name, pod4WithConfig.Pod.Namespace),
					podConfig(pod5WithConfig.Pod.Name, pod5WithConfig.Pod.Namespace),
				)),
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1WithConfig.Pod.Name: pod1WithConfig.Pod,
						pod2WithConfig.Pod.Name: pod2WithConfig.Pod,
						pod3WithConfig.Pod.Name: pod3WithConfig.Pod,
						pod4WithConfig.Pod.Name: pod4WithConfig.Pod,
						pod5WithConfig.Pod.Name: pod5WithConfig.Pod,
					},
				},
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToDelete: pod.PodsWithConfig{
							pod4WithConfig,
							pod5WithConfig,
						},
					},
				},
			},
			assertions: func(t *testing.T, c k8s.Client, performableChanges *mutation.PerformableChanges) {
				assertMinimumMasterNodes(t, 2, c, performableChanges, []corev1.Pod{pod1WithConfig.Pod})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Zen1UpdateMinimumMasterNodesConfig(
				tt.args.c, tt.args.es, tt.args.podsState, tt.args.performableChanges,
			); (err != nil) != tt.wantErr {
				t.Errorf("Zen1UpdateMinimumMasterNodesConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			tt.assertions(t, tt.args.c, tt.args.performableChanges)
		})
	}
}

func assertMinimumMasterNodes(
	t *testing.T,
	minimumMasterNodes int,
	c k8s.Client,
	performableChanges *mutation.PerformableChanges,
	pods []corev1.Pod,
) {
	for _, toCreate := range performableChanges.ToCreate {
		podSettings, err := toCreate.PodSpecCtx.Config.Unpack()
		require.NoError(t, err)
		assert.Equal(t, minimumMasterNodes, podSettings.Discovery.Zen.MinimumMasterNodes)
	}

	for _, pod := range pods {
		cfg, err := settings.GetESConfigContent(c, k8s.ExtractNamespacedName(&pod))
		require.NoError(t, err)

		podSettings, err := cfg.Unpack()
		require.NoError(t, err)
		require.Equal(t, minimumMasterNodes, podSettings.Discovery.Zen.MinimumMasterNodes)
	}
}
