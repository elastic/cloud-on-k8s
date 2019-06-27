// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

type ExpectedESCall struct {
	migrateData             string
	disableShardsAllocation bool
	syncFlush               bool
}

type FakeESClient struct {
	esclient.Client
	calls []ExpectedESCall
}

func (f *FakeESClient) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
	f.calls = append(f.calls, ExpectedESCall{
		migrateData: nodes,
	})
	return nil
}

func (f *FakeESClient) DisableReplicasShardAllocation(ctx context.Context) error {
	f.calls = append(f.calls, ExpectedESCall{
		disableShardsAllocation: true,
	})
	return nil
}

func (f *FakeESClient) SyncedFlush(ctx context.Context) error {
	f.calls = append(f.calls, ExpectedESCall{
		syncFlush: true,
	})
	return nil
}

func withPVCReuse(podsToDelete mutation.PodsToDelete) mutation.PodsToDelete {
	for i := range podsToDelete {
		podsToDelete[i].ReusePVC = true
	}
	return podsToDelete
}

func podsToDelete(fromPods ...corev1.Pod) mutation.PodsToDelete {
	res := make(mutation.PodsToDelete, 0, len(fromPods))
	for _, p := range fromPods {
		res = append(res, mutation.PodToDelete{
			PodWithConfig: pod.PodWithConfig{
				Pod: p,
			},
		})
	}
	return res
}

func TestPodDeletionHandler_HandleDeletions(t *testing.T) {
	volumeClaim1 := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "claim1",
		},
	}
	pod1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod1",
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "volume-name",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: volumeClaim1.Name,
						},
					},
				},
			},
		},
	}
	pod2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod2",
		},
	}
	shardsMigrationInPod2 := observer.State{
		ClusterState: &esclient.ClusterState{
			ClusterName: "test",
			Nodes: map[string]esclient.ClusterStateNode{
				"pod1ID": {Name: pod1.Name},
				"pod2ID": {Name: pod2.Name},
			},
			RoutingTable: esclient.RoutingTable{
				Indices: map[string]esclient.Shards{
					"index0": {
						Shards: map[string][]esclient.Shard{
							"0": {
								{
									Node:  "pod2ID",
									State: "RELOCATING",
								},
							},
						},
					},
				},
			},
		},
	}

	clusterGreen := observer.State{
		ClusterHealth: &esclient.Health{
			Status: string(v1alpha1.ElasticsearchGreenHealth),
		},
	}
	clusterRed := observer.State{
		ClusterHealth: &esclient.Health{
			Status: string(v1alpha1.ElasticsearchRedHealth),
		},
	}

	tests := []struct {
		name                      string
		initialResources          []runtime.Object
		resourcesState            reconcile.ResourcesState
		observedState             observer.State
		performableChanges        *mutation.PerformableChanges
		expectedESCalls           []ExpectedESCall
		wantPods                  []corev1.Pod
		wantPVCs                  []corev1.PersistentVolumeClaim
		wantRequeue               bool
		wantDeletionsExpectations bool
	}{

		{
			name:               "nothing to delete",
			initialResources:   []runtime.Object{&pod1},
			performableChanges: &mutation.PerformableChanges{},
			wantPods:           []corev1.Pod{pod1}, // no pod deleted
		},

		{
			name:             "delete pod 1 and its PVC with data migration",
			initialResources: []runtime.Object{&pod1, &volumeClaim1},
			resourcesState: reconcile.ResourcesState{
				PVCs: []corev1.PersistentVolumeClaim{volumeClaim1},
			},
			observedState: shardsMigrationInPod2,
			performableChanges: &mutation.PerformableChanges{
				Changes: mutation.Changes{
					ToDelete: podsToDelete(pod1),
				},
			},
			expectedESCalls: []ExpectedESCall{
				{migrateData: pod1.Name},
			},
			wantPods:                  []corev1.Pod{},                   // pod deleted
			wantPVCs:                  []corev1.PersistentVolumeClaim{}, // PVC deleted
			wantDeletionsExpectations: true,
		},

		{
			name:             "2 pods to delete with data migration, migration over for pod1 but not pod2",
			initialResources: []runtime.Object{&pod1, &volumeClaim1, &pod2},
			resourcesState: reconcile.ResourcesState{
				PVCs: []corev1.PersistentVolumeClaim{volumeClaim1},
			},
			observedState: shardsMigrationInPod2,
			performableChanges: &mutation.PerformableChanges{
				Changes: mutation.Changes{
					ToDelete: podsToDelete(pod1, pod2),
				},
			},
			expectedESCalls: []ExpectedESCall{
				{migrateData: pod1.Name + "," + pod2.Name},
			},
			wantPods:                  []corev1.Pod{pod2}, // pod1 & pvc deleted
			wantRequeue:               true,               // requeue for pod2
			wantDeletionsExpectations: true,
		},

		{
			name:             "delete pod1 but reuse PVC: skip because cluster isn't ready",
			initialResources: []runtime.Object{&pod1, &volumeClaim1},
			observedState:    clusterRed,
			performableChanges: &mutation.PerformableChanges{
				Changes: mutation.Changes{
					ToDelete: withPVCReuse(podsToDelete(pod1, pod2)),
				},
			},
			expectedESCalls: nil,
			wantPods:        []corev1.Pod{pod1},
			wantPVCs:        []corev1.PersistentVolumeClaim{volumeClaim1},
			wantRequeue:     true,
		},

		{
			name:             "delete pod1 but reuse PVC",
			initialResources: []runtime.Object{&pod1, &volumeClaim1},
			observedState:    clusterGreen,
			performableChanges: &mutation.PerformableChanges{
				Changes: mutation.Changes{
					ToDelete: withPVCReuse(podsToDelete(pod1, pod2)),
				},
			},
			expectedESCalls: []ExpectedESCall{
				{disableShardsAllocation: true},
				{syncFlush: true},
			},
			wantPods:                  []corev1.Pod{},                               // pod deleted
			wantPVCs:                  []corev1.PersistentVolumeClaim{volumeClaim1}, // PVC kept
			wantRequeue:               false,
			wantDeletionsExpectations: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrapClient(fake.NewFakeClient(tt.initialResources...))
			esClient := &FakeESClient{
				Client: esclient.NewMockClient(version.MustParse("7.0.0"), func(req *http.Request) *http.Response {
					return &http.Response{
						StatusCode: 200,
						Body:       ioutil.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
						Request:    req,
					}
				}),
			}
			results := &reconciler.Results{}

			d := &PodDeletionHandler{
				es:                 v1alpha1.Elasticsearch{},
				performableChanges: tt.performableChanges,
				results:            results,
				observedState:      tt.observedState,
				reconcileState:     reconcile.NewState(v1alpha1.Elasticsearch{}),
				esClient:           esClient,
				resourcesState:     &tt.resourcesState,
				defaultDriver: &defaultDriver{
					Options: Options{
						Client:           k8sClient,
						PodsExpectations: reconciler.NewExpectations(),
					},
				},
			}

			// execute the function
			err := d.HandleDeletions()
			require.NoError(t, err)

			// expected ES calls should be made
			require.Equal(t, tt.expectedESCalls, esClient.calls)

			// remaining pods should match
			var pods corev1.PodList
			err = k8sClient.List(&client.ListOptions{}, &pods)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantPods, pods.Items)

			// remaining PVCs should match
			var pvcs corev1.PersistentVolumeClaimList
			err = k8sClient.List(&client.ListOptions{}, &pvcs)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantPVCs, pvcs.Items)

			// result should match
			aggregatedRes, err := results.Aggregate()
			require.NoError(t, err)
			expectedResult := crreconcile.Result{}
			if tt.wantRequeue {
				expectedResult = defaultRequeue
			}
			require.Equal(t, expectedResult, aggregatedRes)

			// expectations should match
			require.Equal(t, tt.wantDeletionsExpectations, !d.defaultDriver.PodsExpectations.Fulfilled(types.NamespacedName{}))
		})
	}
}
