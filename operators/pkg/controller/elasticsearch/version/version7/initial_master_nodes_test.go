// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	defaultClusterUUID = "jiMyMA1hQ-WMPK3vEStZuw"
)

func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Es types")
	}
	return sc
}

var esNN = types.NamespacedName{
	Namespace: "ns1",
	Name:      "foo",
}

func newElasticsearch() *v1alpha1.Elasticsearch {
	return &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNN.Namespace,
			Name:      esNN.Name,
		},
	}
}

func withAnnotation(es *v1alpha1.Elasticsearch, name, value string) *v1alpha1.Elasticsearch {
	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}
	es.Annotations[name] = value
	return es
}

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

	return pod.PodWithConfig{Pod: p, Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()}}
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
	s := setupScheme(t)
	type args struct {
		cluster            *v1alpha1.Elasticsearch
		clusterState       observer.State
		performableChanges mutation.PerformableChanges
		resourcesState     reconcile.ResourcesState
	}
	tests := []struct {
		name                      string
		args                      args
		assertions                func(t *testing.T, changes *mutation.PerformableChanges)
		wantClusterUUIDAnnotation bool
		wantErr                   bool
	}{
		{
			name: "not set when likely already bootstrapped",
			args: args{
				cluster: withAnnotation(newElasticsearch(), ClusterUUIDAnnotationName, defaultClusterUUID),
				clusterState: observer.State{
					ClusterState: &esclient.ClusterState{
						ClusterUUID: defaultClusterUUID,
					},
				},
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true).Pod,
							PodSpecCtx: pod.PodSpecContext{
								Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
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
			wantClusterUUIDAnnotation: true,
		},
		{
			name: "set when likely not bootstrapped",
			args: args{
				cluster:      newElasticsearch(),
				clusterState: observer.State{},
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true).Pod,
							PodSpecCtx: pod.PodSpecContext{
								Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
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
			name: "just been bootstrapped, annotation should be set",
			args: args{
				cluster: newElasticsearch(),
				clusterState: observer.State{
					ClusterState: &esclient.ClusterState{
						ClusterUUID: defaultClusterUUID,
					},
				},
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true).Pod,
							PodSpecCtx: pod.PodSpecContext{
								Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
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
			wantClusterUUIDAnnotation: true,
		},
		{
			name: "all masters are informed of all masters",
			args: args{
				cluster: newElasticsearch(),
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{
							{
								Pod: newPod("b", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
								},
							},
							{
								Pod: newPod("c", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
								},
							},
							{
								Pod: newPod("d", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
								},
							},
							{
								Pod: newPod("e", true).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
								},
							},
							// f is not master, so masters should not be informed of it
							{
								Pod: newPod("f", false).Pod,
								PodSpecCtx: pod.PodSpecContext{
									Config: settings.CanonicalConfig{CanonicalConfig: common.NewCanonicalConfig()},
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
			client := k8s.WrapClient(fake.NewFakeClientWithScheme(s, tt.args.cluster))
			got, err := ClusterInitialMasterNodesEnforcer(
				*tt.args.cluster,
				tt.args.clusterState,
				client,
				tt.args.performableChanges,
				tt.args.resourcesState,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("ClusterInitialMasterNodesEnforcer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var es v1alpha1.Elasticsearch
			err = client.Get(esNN, &es)
			assert.NoError(t, err)
			annotation := es.Annotations != nil && len(es.Annotations[ClusterUUIDAnnotationName]) > 0
			assert.Equal(t, tt.wantClusterUUIDAnnotation, annotation)

			tt.assertions(t, got)
		})
	}
}
