// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newPod creates a new named potentially labeled as master
func newPod(name string, master bool) pod.PodWithConfig {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{}},
		},
	}

	label.NodeTypesMasterLabelName.Set(master, p.Labels)

	return pod.PodWithConfig{Pod: p, Config: settings.NewFlatConfig()}
}

func assertInitialMasterNodes(t *testing.T, changes *mutation.PerformableChanges, shouldBeSet bool, nodeNames ...string) {
	for _, change := range changes.ToCreate {
		cfg, err := change.PodSpecCtx.Config.Unpack()
		require.NoError(t, err)
		nodes := cfg.Cluster.InitialMasterNodes
		if !label.IsMasterNode(change.Pod) {
			require.Nil(t, nodes)
		} else if !shouldBeSet {
			require.Nil(t, nodes)
		} else {
			require.NotNil(t, nodes)
			require.Equal(t, nodeNames, nodes)
		}
	}
}

func TestClusterInitialMasterNodesEnforcer(t *testing.T) {
	type args struct {
		performableChanges mutation.PerformableChanges
		resourcesState     reconcile.ResourcesState
	}
	tests := []struct {
		name       string
		args       args
		assertions func(t *testing.T, changes *mutation.PerformableChanges)
		wantErr    bool
	}{
		{
			name: "not set when likely already bootstrapped",
			args: args{
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true).Pod,
							PodSpecCtx: pod.PodSpecContext{
								Config: settings.NewFlatConfig(),
							},
						}},
					},
				},
				resourcesState: reconcile.ResourcesState{
					CurrentPods: pod.PodsWithConfig{newPod("a", true)},
				},
			},
			assertions: func(t *testing.T, changes *mutation.PerformableChanges) {
				assertInitialMasterNodes(t, changes, false)
			},
		},
		{
			name: "set when likely not bootstrapped",
			args: args{
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true).Pod,
							PodSpecCtx: pod.PodSpecContext{
								Config: settings.NewFlatConfig(),
							},
						}},
					},
				},
				resourcesState: reconcile.ResourcesState{
					CurrentPods: pod.PodsWithConfig{newPod("a", false)},
				},
			},
			assertions: func(t *testing.T, changes *mutation.PerformableChanges) {
				assertInitialMasterNodes(t, changes, true, "b")
			},
		},
		{
			name: "all masters are informed of all masters",
			args: args{
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{
							{
								Pod: newPod("b", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.NewFlatConfig(),
								},
							},
							{
								Pod: newPod("c", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.NewFlatConfig(),
								},
							},
							{
								Pod: newPod("d", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.NewFlatConfig(),
								},
							},
							{
								Pod: newPod("e", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.NewFlatConfig(),
								},
							},
							// f is not master, so masters should not be informed of it
							{
								Pod: newPod("f", false).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.NewFlatConfig(),
								},
							},
						},
					},
				},
			},
			assertions: func(t *testing.T, changes *mutation.PerformableChanges) {
				assertInitialMasterNodes(t, changes, true, "b", "c", "d", "e")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClusterInitialMasterNodesEnforcer(tt.args.performableChanges, tt.args.resourcesState)
			if (err != nil) != tt.wantErr {
				t.Errorf("ClusterInitialMasterNodesEnforcer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tt.assertions(t, got)
		})
	}
}
