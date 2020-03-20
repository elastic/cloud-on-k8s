// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	commonscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Sample StatefulSets to use in tests
var (
	clusterName = "cluster-name"
	es          = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: "ns",
		},
	}
	ssetMaster3Replicas = sset.TestSset{
		Name:      "ssetMaster3Replicas",
		Namespace: "ns",
		Version:   "7.2.0",
		Replicas:  3,
		Master:    true,
		Data:      false,
	}.Build()
	podsSsetMaster3Replicas = []corev1.Pod{
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            sset.PodName(ssetMaster3Replicas.Name, 0),
			StatefulSetName: ssetMaster3Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Master:          true,
			Ready:           true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            sset.PodName(ssetMaster3Replicas.Name, 1),
			StatefulSetName: ssetMaster3Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Master:          true,
			Ready:           true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            sset.PodName(ssetMaster3Replicas.Name, 2),
			StatefulSetName: ssetMaster3Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Master:          true,
			Ready:           true,
		}.Build(),
	}
	ssetData4Replicas = sset.TestSset{
		Name:      "ssetData4Replicas",
		Namespace: "ns",
		Version:   "7.2.0",
		Replicas:  4,
		Master:    false,
		Data:      true,
	}.Build()
	podsSsetData4Replicas = []corev1.Pod{
		sset.TestPod{
			Namespace:       ssetData4Replicas.Namespace,
			Name:            sset.PodName(ssetData4Replicas.Name, 0),
			StatefulSetName: ssetData4Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Data:            true,
			Ready:           true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetData4Replicas.Namespace,
			Name:            sset.PodName(ssetData4Replicas.Name, 1),
			StatefulSetName: ssetData4Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Data:            true,
			Ready:           true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetData4Replicas.Namespace,
			Name:            sset.PodName(ssetData4Replicas.Name, 2),
			StatefulSetName: ssetData4Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Data:            true,
			Ready:           true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetData4Replicas.Namespace,
			Name:            sset.PodName(ssetData4Replicas.Name, 3),
			StatefulSetName: ssetData4Replicas.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Data:            true,
			Ready:           true,
		}.Build(),
	}
	runtimeObjs = []runtime.Object{&es, &ssetMaster3Replicas, &ssetData4Replicas,
		&podsSsetMaster3Replicas[0], &podsSsetMaster3Replicas[1], &podsSsetMaster3Replicas[2],
		&podsSsetData4Replicas[0], &podsSsetData4Replicas[1], &podsSsetData4Replicas[2], &podsSsetData4Replicas[3],
	}
	requeueResults = (&reconciler.Results{}).WithResult(defaultRequeue)
	emptyResults   = &reconciler.Results{}
)

// -- Tests start here

func TestHandleDownscale(t *testing.T) {
	// This test focuses on one code path that visits most functions.
	// Derived paths are individually tested in unit tests of the other functions.

	// We want to downscale 2 StatefulSets (masters 3 -> 1 and data 4 -> 2) in version 7.X,
	// but should only be allowed a partial downscale (3 -> 2 and 4 -> 3).

	k8sClient := k8s.WrappedFakeClient(runtimeObjs...)
	esClient := &fakeESClient{}
	actualStatefulSets := sset.StatefulSetList{ssetMaster3Replicas, ssetData4Replicas}
	downscaleCtx := downscaleContext{
		k8sClient:      k8sClient,
		expectations:   expectations.NewExpectations(k8sClient),
		reconcileState: reconcile.NewState(esv1.Elasticsearch{}),
		shardLister: migration.NewFakeShardLister(
			esclient.Shards{
				{Index: "index-1", Shard: "0", State: esclient.STARTED, NodeName: "ssetData4Replicas-2"},
			},
		),
		esClient:  esClient,
		es:        es,
		parentCtx: context.Background(),
	}

	// request master nodes downscale from 3 to 1 replicas
	ssetMaster3ReplicasDownscaled := *ssetMaster3Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetMaster3ReplicasDownscaled, pointer.Int32(1))
	// request data nodes downscale from 4 to 2 replicas
	ssetData4ReplicasDownscaled := *ssetData4Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetData4ReplicasDownscaled, pointer.Int32(2))
	requestedStatefulSets := sset.StatefulSetList{ssetMaster3ReplicasDownscaled, ssetData4ReplicasDownscaled}

	// do the downscale
	results := HandleDownscale(downscaleCtx, requestedStatefulSets, actualStatefulSets)
	require.False(t, results.HasError())

	// data migration should have been requested for all nodes, but last master, leaving the cluster
	require.True(t, esClient.ExcludeFromShardAllocationCalled)
	require.Equal(t, "ssetMaster3Replicas-2,ssetData4Replicas-3,ssetData4Replicas-2", esClient.ExcludeFromShardAllocationCalledWith)

	// only part of the expected replicas of ssetMaster3Replicas should be updated,
	// since we remove only one master at a time
	ssetMaster3ReplicasExpectedAfterDownscale := *ssetMaster3Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetMaster3ReplicasExpectedAfterDownscale, pointer.Int32(2))
	// only part of the expected replicas of ssetData4Replicas should be updated,
	// since a node still needs to migrate data
	ssetData4ReplicasExpectedAfterDownscale := *ssetData4Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetData4ReplicasExpectedAfterDownscale, pointer.Int32(3))

	expectedAfterDownscale := []appsv1.StatefulSet{ssetMaster3ReplicasExpectedAfterDownscale, ssetData4ReplicasExpectedAfterDownscale}

	// a requeue should be requested since all nodes were not downscaled
	// (2 requeues actually: for data migration & master nodes)
	require.Equal(t, (&reconciler.Results{}).WithResult(defaultRequeue).WithResult(defaultRequeue), results)

	// voting config exclusion should have been added for leaving masters
	require.True(t, esClient.AddVotingConfigExclusionsCalled)
	require.Equal(t, []string{"ssetMaster3Replicas-2"}, esClient.AddVotingConfigExclusionsCalledWith)

	// compare what has been updated in the apiserver with what we would expect
	var actual appsv1.StatefulSetList
	err := k8sClient.List(&actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// simulate pods deletion that would be done by the StatefulSet controller
	require.NoError(t, k8sClient.Delete(&podsSsetMaster3Replicas[2]))
	require.NoError(t, k8sClient.Delete(&podsSsetData4Replicas[3]))

	// running the downscale again should remove the next master,
	// and also requeue since data migration is still not over for data nodes
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, requeueResults, results)

	// one less master
	nodespec.UpdateReplicas(&ssetMaster3ReplicasExpectedAfterDownscale, pointer.Int32(1))
	expectedAfterDownscale = []appsv1.StatefulSet{ssetMaster3ReplicasExpectedAfterDownscale, ssetData4ReplicasExpectedAfterDownscale}
	err = k8sClient.List(&actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// simulate master pod deletion
	require.NoError(t, k8sClient.Delete(&podsSsetMaster3Replicas[1]))

	// once data migration is over the downscale should continue for next data nodes
	downscaleCtx.shardLister = migration.NewFakeShardLister(
		esclient.Shards{
			{Index: "index-1", Shard: "0", State: esclient.STARTED, NodeName: "ssetData4Replicas-1"},
		},
	)
	nodespec.UpdateReplicas(&expectedAfterDownscale[1], pointer.Int32(2))
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, emptyResults, results)
	err = k8sClient.List(&actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// data migration should have been requested for the data node leaving the cluster
	require.True(t, esClient.ExcludeFromShardAllocationCalled)
	require.Equal(t, "ssetData4Replicas-2", esClient.ExcludeFromShardAllocationCalledWith)

	// simulate pod deletion
	require.NoError(t, k8sClient.Delete(&podsSsetData4Replicas[2]))

	// running the downscale again should not remove any new node
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, emptyResults, results)
	err = k8sClient.List(&actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// data migration settings should have been cleared
	require.True(t, esClient.ExcludeFromShardAllocationCalled)
	require.Equal(t, "none_excluded", esClient.ExcludeFromShardAllocationCalledWith)

	// simulate the existence of a third StatefulSet with data nodes
	// that we want to remove
	ssetToRemove := sset.TestSset{
		Name:      "ssetToRemove",
		Namespace: "ns",
		Version:   "7.2.0",
		Replicas:  2,
		Master:    false,
		Data:      true,
	}.Build()
	require.NoError(t, k8sClient.Create(&ssetToRemove))
	// do the downscale, that third StatefulSet is not part of the expected ones
	err = k8sClient.List(&actual)
	require.NoError(t, err)
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	// the StatefulSet replicas should be decreased to 0, but StatefulSet should still be around
	err = k8sClient.Get(k8s.ExtractNamespacedName(&ssetToRemove), &ssetToRemove)
	require.NoError(t, err)
	require.Equal(t, int32(0), sset.GetReplicas(ssetToRemove))
	// run downscale again: this time the StatefulSet should be removed
	err = k8sClient.List(&actual)
	require.NoError(t, err)
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	err = k8sClient.Get(k8s.ExtractNamespacedName(&ssetToRemove), &ssetToRemove)
	require.True(t, apierrors.IsNotFound(err))
}

func Test_calculateDownscales(t *testing.T) {
	ssets := sset.StatefulSetList{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset0",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32(3),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset1",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32(3)},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset2",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32(3)},
		},
	}

	var tests = []struct {
		name                 string
		expectedStatefulSets sset.StatefulSetList
		actualStatefulSets   sset.StatefulSetList
		wantDownscales       []ssetDownscale
		wantDeletions        sset.StatefulSetList
	}{
		{
			name:               "no actual statefulset: nothing to do",
			actualStatefulSets: nil,
			wantDownscales:     nil,
			wantDeletions:      nil,
		},

		{
			name: "upscale: nothing to do",
			expectedStatefulSets: sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset0",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(4),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset1",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(5)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset2",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(3)},
				},
			},
			actualStatefulSets: ssets,
			wantDownscales:     nil,
			wantDeletions:      nil,
		},
		{
			name:                 "expected == actual",
			expectedStatefulSets: ssets,
			actualStatefulSets:   ssets,
			wantDownscales:       nil,
			wantDeletions:        nil,
		},
		{
			name:                 "downscale all ssets",
			expectedStatefulSets: nil,
			actualStatefulSets:   ssets,
			wantDownscales: []ssetDownscale{
				{
					statefulSet:     ssets[0],
					initialReplicas: *ssets[0].Spec.Replicas,
					targetReplicas:  0,
				},
				{
					statefulSet:     ssets[1],
					initialReplicas: *ssets[1].Spec.Replicas,
					targetReplicas:  0,
				},
				{
					statefulSet:     ssets[2],
					initialReplicas: *ssets[2].Spec.Replicas,
					targetReplicas:  0,
				},
			},
			// should not delete any statefulset, only downscale existing ones to 0
			wantDeletions: nil,
		},
		{
			name: "downscale 2 out of 3 StatefulSets",
			expectedStatefulSets: sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset0",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(3),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset1",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(2)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset2",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(1)},
				},
			},
			actualStatefulSets: ssets,
			wantDownscales: []ssetDownscale{
				{
					statefulSet:     ssets[1],
					initialReplicas: *ssets[1].Spec.Replicas,
					targetReplicas:  2,
					finalReplicas:   2,
				},
				{
					statefulSet:     ssets[2],
					initialReplicas: *ssets[2].Spec.Replicas,
					targetReplicas:  1,
					finalReplicas:   1,
				},
			},
			wantDeletions: nil,
		},
		{
			name: "delete actual statefulsets with 0 replicas",
			expectedStatefulSets: sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset2",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(1)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset3",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
			},
			actualStatefulSets: sset.StatefulSetList{
				// statefulset with 0 replicas which has no corresponding expected statefulset: should be deleted
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset1",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
				// statefulset with 0 replicas which has a corresponding expected statefulset with 1 replica: should be kept
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset2",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
				// statefulset with 1 replicas that should be downscaled to 0
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset3",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(1)},
				},
			},
			wantDownscales: []ssetDownscale{
				{
					statefulSet: appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      "sset3",
						},
						Spec: appsv1.StatefulSetSpec{
							Replicas: pointer.Int32(1)},
					},
					initialReplicas: 1,
					targetReplicas:  0,
					finalReplicas:   0,
				},
			},
			wantDeletions: sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "sset1",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDownscales, gotDeletions := calculateDownscales(downscaleState{}, tt.expectedStatefulSets, tt.actualStatefulSets)
			require.Equal(t, tt.wantDownscales, gotDownscales)
			require.Equal(t, tt.wantDeletions, gotDeletions)
		})
	}
}

func Test_calculatePerformableDownscale(t *testing.T) {
	type args struct {
		ctx       downscaleContext
		downscale ssetDownscale
		state     *downscaleState
	}
	tests := []struct {
		name    string
		args    args
		want    ssetDownscale
		wantErr bool
	}{
		{
			name: "no downscale planned",
			args: args{
				ctx: downscaleContext{},
				downscale: ssetDownscale{
					initialReplicas: 3,
					targetReplicas:  3,
					finalReplicas:   3,
				},
				state: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: pointer.Int32(1)},
			},
			want: ssetDownscale{
				initialReplicas: 3,
				targetReplicas:  3,
				finalReplicas:   3,
			},
		},
		{
			name: "downscale possible from 3 to 2",
			args: args{
				ctx: downscaleContext{
					shardLister: migration.NewFakeShardLister(esclient.Shards{}),
				},
				downscale: ssetDownscale{
					initialReplicas: 3,
					targetReplicas:  2,
					finalReplicas:   2,
				},
				state: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: pointer.Int32(1)},
			},
			want: ssetDownscale{
				initialReplicas: 3,
				targetReplicas:  2,
				finalReplicas:   2,
			},
		},
		{
			name: "downscale not possible from 3 to 2 (would violate maxUnavailable)",
			args: args{
				ctx: downscaleContext{
					observedState: observer.State{},
					shardLister:   migration.NewFakeShardLister(esclient.Shards{}),
				},
				downscale: ssetDownscale{
					initialReplicas: 3,
					targetReplicas:  3,
					finalReplicas:   2,
				},
				state: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: pointer.Int32(0)},
			},
			want: ssetDownscale{
				initialReplicas: 3,
				targetReplicas:  3,
				finalReplicas:   2,
			},
		},
		{
			name: "downscale not possible: one master already removed",
			args: args{
				ctx: downscaleContext{
					shardLister: migration.NewFakeShardLister(esclient.Shards{}),
				},
				downscale: ssetDownscale{
					statefulSet:     ssetMaster3Replicas,
					initialReplicas: 3,
					targetReplicas:  3,
					finalReplicas:   2,
				},
				// a master node has already been removed
				state: &downscaleState{masterRemovalInProgress: true, runningMasters: 3, removalsAllowed: pointer.Int32(1)},
			},
			want: ssetDownscale{
				statefulSet:     ssetMaster3Replicas,
				initialReplicas: 3,
				targetReplicas:  3,
				finalReplicas:   2,
			},
		},
		{
			name: "downscale only possible from 3 to 2 instead of 3 to 1 (1 master at a time)",
			args: args{
				ctx: downscaleContext{
					shardLister: migration.NewFakeShardLister(esclient.Shards{}),
				},
				downscale: ssetDownscale{
					statefulSet:     ssetMaster3Replicas,
					initialReplicas: 3,
					targetReplicas:  2,
					finalReplicas:   1,
				},
				// invariants limits us to one master node downscale only
				state: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: pointer.Int32(1)},
			},
			want: ssetDownscale{
				statefulSet:     ssetMaster3Replicas,
				initialReplicas: 3,
				targetReplicas:  2,
				finalReplicas:   1,
			},
		},
		{
			name: "downscale not possible: cannot remove the last master",
			args: args{
				ctx: downscaleContext{
					shardLister: migration.NewFakeShardLister(esclient.Shards{}),
				},
				downscale: ssetDownscale{
					statefulSet:     ssetMaster3Replicas,
					initialReplicas: 1,
					targetReplicas:  1,
					finalReplicas:   0,
				},
				// only one master is running
				state: &downscaleState{masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: pointer.Int32(1)},
			},
			want: ssetDownscale{
				statefulSet:     ssetMaster3Replicas,
				initialReplicas: 1,
				targetReplicas:  1,
				finalReplicas:   0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculatePerformableDownscale(tt.args.ctx, tt.args.downscale)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculatePerformableDownscale() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculatePerformableDownscale() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_attemptDownscale(t *testing.T) {
	tests := []struct {
		name                 string
		downscale            ssetDownscale
		state                *downscaleState
		statefulSets         sset.StatefulSetList
		expectedStatefulSets []appsv1.StatefulSet
	}{
		{
			name: "perform 3 -> 2 downscale",
			downscale: ssetDownscale{
				statefulSet:     sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
				initialReplicas: 3,
				targetReplicas:  2,
			},
			state: &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(1)},
			statefulSets: sset.StatefulSetList{
				sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
			},
			expectedStatefulSets: []appsv1.StatefulSet{
				sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 2, Master: true, Data: true}.Build(),
			},
		},
		{
			name: "try perform 3 -> 2 downscale, but stay at 3 due to maxUnavailable",
			downscale: ssetDownscale{
				statefulSet:     sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
				initialReplicas: 3,
				targetReplicas:  3,
				finalReplicas:   2,
			},
			state: &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(0)},
			statefulSets: sset.StatefulSetList{
				sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
			},
			expectedStatefulSets: []appsv1.StatefulSet{
				sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runtimeObjs []runtime.Object
			for i := range tt.statefulSets {
				runtimeObjs = append(runtimeObjs, &tt.statefulSets[i])
			}
			k8sClient := k8s.WrappedFakeClient(runtimeObjs...)
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient),
				reconcileState: reconcile.NewState(esv1.Elasticsearch{}),
				shardLister:    migration.NewFakeShardLister(esclient.Shards{}),
				esClient:       &fakeESClient{},
			}
			// do the downscale
			_, err := attemptDownscale(downscaleCtx, tt.downscale, tt.statefulSets)
			require.NoError(t, err)
			// retrieve statefulsets
			var ssets appsv1.StatefulSetList
			err = k8sClient.List(&ssets)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedStatefulSets), len(ssets.Items))
			for i := range tt.expectedStatefulSets {
				comparison.AssertEqual(t, &tt.expectedStatefulSets[i], &ssets.Items[i])
			}
		})
	}
}

func Test_doDownscale_updateReplicasAndExpectations(t *testing.T) {
	sset1 := ssetMaster3Replicas
	sset1.Generation = 1
	sset2 := ssetData4Replicas
	sset2.Generation = 1
	k8sClient := k8s.WrappedFakeClient(&sset1, &sset2)
	downscaleCtx := downscaleContext{
		k8sClient:    k8sClient,
		expectations: expectations.NewExpectations(k8sClient),
		esClient:     &fakeESClient{},
	}

	expectedSset1 := *sset1.DeepCopy()
	// simulate sset generation updated during the downscale (not done by the fake client)
	sset1.Generation = 2
	expectedSset1.Generation = 2
	// downscale a StatefulSet from 3 to 2 replicas
	downscale := ssetDownscale{
		statefulSet:     sset1,
		initialReplicas: 3,
		targetReplicas:  2,
	}
	nodespec.UpdateReplicas(&expectedSset1, &downscale.targetReplicas)

	// no expectation is currently set
	require.Len(t, downscaleCtx.expectations.GetGenerations(), 0)

	// do the downscale
	err := doDownscale(downscaleCtx, downscale, sset.StatefulSetList{sset1, sset2})
	require.NoError(t, err)

	// sset resource should be updated
	var ssets appsv1.StatefulSetList
	err = k8sClient.List(&ssets)
	require.NoError(t, err)
	expectedSsets := []appsv1.StatefulSet{expectedSset1, sset2}
	require.Equal(t, len(expectedSsets), len(ssets.Items))
	for i := range expectedSsets {
		comparison.AssertEqual(t, &expectedSsets[i], &ssets.Items[i])
	}

	// expectations should have been be registered
	require.Len(t, downscaleCtx.expectations.GetGenerations(), 1)
}

func Test_doDownscale_zen2VotingConfigExclusions(t *testing.T) {
	ssetMasters := sset.TestSset{
		Name:        "masters",
		Namespace:   es.Namespace,
		ClusterName: es.Name,
		Version:     "7.1.0",
		Replicas:    3,
		Master:      true,
		Data:        false,
	}.Build()
	ssetData := sset.TestSset{
		Name:        "datas",
		Namespace:   es.Namespace,
		ClusterName: es.Name,
		Version:     "7.1.0",
		Replicas:    3,
		Master:      false,
		Data:        true,
	}.Build()
	tests := []struct {
		name               string
		downscale          ssetDownscale
		wantZen2Called     bool
		wantZen2CalledWith []string
	}{
		{
			name: "3 -> 2 master nodes",
			downscale: ssetDownscale{
				statefulSet:     ssetMasters,
				initialReplicas: 3,
				targetReplicas:  2,
			},
			wantZen2Called:     true,
			wantZen2CalledWith: []string{"masters-2"},
		},
		{
			name: "3 -> 2 data nodes",
			downscale: ssetDownscale{
				statefulSet:     ssetData,
				initialReplicas: 3,
				targetReplicas:  2,
			},
			wantZen2Called:     false,
			wantZen2CalledWith: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// simulate an existing v7 master for zen2 to be called
			v7Pod := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: es.Namespace,
					Labels: map[string]string{
						label.ClusterNameLabelName:             es.Name,
						string(label.NodeTypesMasterLabelName): "true",
						label.VersionLabelName:                 "7.1.0",
						label.StatefulSetNameLabelName:         ssetMasters.Name,
					},
				},
			}
			k8sClient := k8s.WrappedFakeClient(es.DeepCopy(), &ssetMasters, &ssetData, &v7Pod)
			esClient := &fakeESClient{}
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient),
				reconcileState: reconcile.NewState(esv1.Elasticsearch{}),
				esClient:       esClient,
				es:             es,
				parentCtx:      context.Background(),
			}
			// do the downscale
			err := doDownscale(downscaleCtx, tt.downscale, sset.StatefulSetList{ssetMasters, ssetData})
			require.NoError(t, err)
			// check call to zen2 is the expected one
			require.Equal(t, tt.wantZen2Called, esClient.AddVotingConfigExclusionsCalled)
			require.Equal(t, tt.wantZen2CalledWith, esClient.AddVotingConfigExclusionsCalledWith)
			// check zen1 was not called
			require.False(t, esClient.SetMinimumMasterNodesCalled)
		})
	}
}

func Test_doDownscale_zen1MinimumMasterNodes(t *testing.T) {
	require.NoError(t, commonscheme.SetupScheme())
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: ssetMaster3Replicas.Namespace, Name: "es"}}
	ssetMasters := sset.TestSset{Name: "masters", Version: "6.8.0", Replicas: 3, Master: true, Data: false}.Build()
	masterPods := []corev1.Pod{
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            ssetMaster3Replicas.Name + "-0",
			ClusterName:     es.Name,
			StatefulSetName: ssetMaster3Replicas.Name,
			Version:         "6.8.0",
			Master:          true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            ssetMaster3Replicas.Name + "-1",
			ClusterName:     es.Name,
			StatefulSetName: ssetMaster3Replicas.Name,
			Version:         "6.8.0",
			Master:          true,
		}.Build(),
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            ssetMaster3Replicas.Name + "-2",
			ClusterName:     es.Name,
			StatefulSetName: ssetMaster3Replicas.Name,
			Version:         "6.8.0",
			Master:          true,
		}.Build(),
	}
	ssetData := sset.TestSset{Name: "datas", Version: "6.8.0", Replicas: 3, Master: false, Data: true}.Build()
	tests := []struct {
		name               string
		downscale          ssetDownscale
		statefulSets       sset.StatefulSetList
		apiserverResources []runtime.Object
		wantZen1Called     bool
		wantZen1CalledWith int
	}{
		{
			name: "3 -> 2 master nodes",
			downscale: ssetDownscale{
				statefulSet:     ssetMasters,
				initialReplicas: 3,
				targetReplicas:  2,
			},
			statefulSets:       sset.StatefulSetList{ssetMasters},
			apiserverResources: []runtime.Object{&es, &ssetMasters, &masterPods[0], &masterPods[1], &masterPods[2]},
			wantZen1Called:     false,
		},
		{
			name: "3 -> 2 data nodes",
			downscale: ssetDownscale{
				statefulSet:     ssetData,
				initialReplicas: 3,
				targetReplicas:  2,
			},
			statefulSets:       sset.StatefulSetList{ssetMasters, ssetData},
			apiserverResources: []runtime.Object{&es, &ssetMasters, &ssetData, &masterPods[0], &masterPods[1], &masterPods[2]},
			wantZen1Called:     false,
		},
		{
			name: "2 -> 1 master nodes",
			downscale: ssetDownscale{
				statefulSet:     ssetMasters,
				initialReplicas: 2,
				targetReplicas:  1,
			},
			statefulSets: sset.StatefulSetList{ssetMasters},
			// 2 master nodes in the apiserver
			apiserverResources: []runtime.Object{&es, &ssetMasters, &masterPods[0], &masterPods[1]},
			wantZen1Called:     true,
			wantZen1CalledWith: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(tt.apiserverResources...)
			esClient := &fakeESClient{}
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient),
				reconcileState: reconcile.NewState(esv1.Elasticsearch{}),
				esClient:       esClient,
				es:             es,
				parentCtx:      context.Background(),
			}
			// do the downscale
			err := doDownscale(downscaleCtx, tt.downscale, tt.statefulSets)
			require.NoError(t, err)
			// check call to zen1 is the expected one
			require.Equal(t, tt.wantZen1Called, esClient.SetMinimumMasterNodesCalled)
			require.Equal(t, tt.wantZen1CalledWith, esClient.SetMinimumMasterNodesCalledWith)
			// check zen2 was not called
			require.False(t, esClient.AddVotingConfigExclusionsCalled)
		})
	}
}

func Test_deleteStatefulSetResources(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"}}
	sset := sset.TestSset{Namespace: "ns", Name: "sset", ClusterName: es.Name}.Build()
	cfg := settings.ConfigSecret(es, sset.Name, []byte("fake config data"))
	svc := nodespec.HeadlessService(k8s.ExtractNamespacedName(&es), sset.Name)

	tests := []struct {
		name      string
		resources []runtime.Object
	}{
		{
			name:      "happy path: delete 3 resources",
			resources: []runtime.Object{&es, &sset, &cfg, &svc},
		},
		{
			name:      "cfg and service were already deleted: should not return an error",
			resources: []runtime.Object{&es, &sset},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(tt.resources...)
			err := deleteStatefulSetResources(k8sClient, es, sset)
			require.NoError(t, err)
			// sset, cfg and headless services should not be there anymore
			require.True(t, apierrors.IsNotFound(k8sClient.Get(k8s.ExtractNamespacedName(&sset), &sset)))
			require.True(t, apierrors.IsNotFound(k8sClient.Get(k8s.ExtractNamespacedName(&cfg), &cfg)))
			require.True(t, apierrors.IsNotFound(k8sClient.Get(k8s.ExtractNamespacedName(&svc), &svc)))
		})
	}
}

func Test_deleteStatefulSets(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"}}
	tests := []struct {
		name          string
		toDelete      sset.StatefulSetList
		objs          []runtime.Object
		wantRemaining sset.StatefulSetList
		wantErr       func(err error) bool
	}{
		{
			name:     "nothing to delete",
			toDelete: nil,
			objs: []runtime.Object{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name}.BuildPtr(),
			},
			wantRemaining: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.Build(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name}.Build(),
			},
		},
		{
			name: "two StatefulSets to delete",
			toDelete: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.Build(),
				sset.TestSset{Namespace: "ns", Name: "sset3", ClusterName: es.Name}.Build(),
			},
			objs: []runtime.Object{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset3", ClusterName: es.Name}.BuildPtr(),
			},
			wantRemaining: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name}.Build(),
			},
		},
		{
			name: "statefulSet already deleted",
			toDelete: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.Build(),
			},
			objs:          []runtime.Object{},
			wantRemaining: nil,
			wantErr:       apierrors.IsNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrappedFakeClient(tt.objs...)
			err := deleteStatefulSets(tt.toDelete, client, es)
			if tt.wantErr != nil {
				require.True(t, tt.wantErr(err))
			} else {
				require.NoError(t, err)
			}
			var remainingSsets appsv1.StatefulSetList
			require.NoError(t, client.List(&remainingSsets))
			require.Equal(t, tt.wantRemaining, sset.StatefulSetList(remainingSsets.Items))
		})
	}
}
