// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
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
	ssetMaster1Replica = sset.TestSset{
		Name:      "ssetMaster1Replicas",
		Namespace: "ns",
		Version:   "7.2.0",
		Replicas:  1,
		Master:    true,
		Data:      false,
	}.Build()
	podsSsetMaster1Replica = []corev1.Pod{
		sset.TestPod{
			Namespace:       ssetMaster3Replicas.Namespace,
			Name:            sset.PodName(ssetMaster1Replica.Name, 0),
			StatefulSetName: ssetMaster1Replica.Name,
			ClusterName:     clusterName,
			Version:         "7.2.0",
			Master:          true,
			Ready:           true,
		}.Build(),
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
	runtimeObjs = []runtime.Object{&es, &ssetMaster1Replica, &ssetMaster3Replicas, &ssetData4Replicas,
		&podsSsetMaster1Replica[0], &podsSsetMaster3Replicas[0], &podsSsetMaster3Replicas[1], &podsSsetMaster3Replicas[2],
		&podsSsetData4Replicas[0], &podsSsetData4Replicas[1], &podsSsetData4Replicas[2], &podsSsetData4Replicas[3],
	}
	emptyResults = &reconciler.Results{}
)

// -- Tests start here

func TestHandleDownscale(t *testing.T) {
	// This test focuses on one code path that visits most functions.
	// Derived paths are individually tested in unit tests of the other functions.

	// We want to downscale 3 StatefulSets (masters 1-> 0, masters 3 -> 1 and data 4 -> 2) in version 7.X,
	// but should only be allowed a partial downscale (1->0, 3 -> 3 and 4 -> 3).

	k8sClient := k8s.NewFakeClient(runtimeObjs...)
	esClient := &fakeESClient{}
	actualStatefulSets := sset.StatefulSetList{ssetMaster1Replica, ssetMaster3Replicas, ssetData4Replicas}
	shardLister := migration.NewFakeShardLister(
		esclient.Shards{
			{Index: "index-1", Shard: "0", State: esclient.STARTED, NodeName: "ssetData4Replicas-2"},
		},
	)
	reconcileState := reconcile.MustNewState(esv1.Elasticsearch{})
	downscaleCtx := downscaleContext{
		k8sClient:      k8sClient,
		expectations:   expectations.NewExpectations(k8sClient),
		reconcileState: reconcileState,
		nodeShutdown:   shutdown.WithObserver(migration.NewShardMigration(es, esClient, shardLister), reconcileState),
		esClient:       esClient,
		es:             es,
		parentCtx:      context.Background(),
	}

	// request master nodes downscale from 3 to 1 replicas the other master StatefulSet should not be there anymore
	ssetMaster3ReplicasDownscaled := *ssetMaster3Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetMaster3ReplicasDownscaled, pointer.Int32(1))
	// request data nodes downscale from 4 to 2 replicas
	ssetData4ReplicasDownscaled := *ssetData4Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetData4ReplicasDownscaled, pointer.Int32(2))
	requestedStatefulSets := sset.StatefulSetList{ssetMaster3ReplicasDownscaled, ssetData4ReplicasDownscaled}

	// do the downscale
	results := HandleDownscale(downscaleCtx, requestedStatefulSets, actualStatefulSets)
	require.False(t, results.HasError())

	// data migration should have been requested for all nodes, but three master in Master3 sset, leaving the cluster
	require.True(t, esClient.ExcludeFromShardAllocationCalled)
	require.Equal(t, "ssetMaster1Replicas-0,ssetData4Replicas-3,ssetData4Replicas-2", esClient.ExcludeFromShardAllocationCalledWith)

	// status should reflect the in progress operations
	require.Equal(t,
		[]esv1.DownscaledNode{
			{Name: "ssetData4Replicas-2", ShutdownStatus: "IN_PROGRESS"},
			{Name: "ssetData4Replicas-3", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster1Replicas-0", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-1", ShutdownStatus: "NOT_STARTED"},
			{Name: "ssetMaster3Replicas-2", ShutdownStatus: "NOT_STARTED"},
		},
		reconcileState.MergeStatusReportingWith(esv1.ElasticsearchStatus{}).DownscaleOperation.Nodes,
	)

	// only part of the expected replicas of ssetMaster1Replicas should be updated,
	// since we remove only one master at a time
	ssetMaster1ReplicaExpectedAfterDownscale := *ssetMaster1Replica.DeepCopy()
	nodespec.UpdateReplicas(&ssetMaster1ReplicaExpectedAfterDownscale, pointer.Int32(0))

	// only part of the expected replicas of ssetData4Replicas should be updated,
	// since a node still needs to migrate data
	ssetData4ReplicasExpectedAfterDownscale := *ssetData4Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetData4ReplicasExpectedAfterDownscale, pointer.Int32(3))

	expectedAfterDownscale := []appsv1.StatefulSet{ssetData4ReplicasExpectedAfterDownscale, ssetMaster1ReplicaExpectedAfterDownscale, ssetMaster3Replicas}

	// a requeue should be requested since all nodes were not downscaled
	// (2 requeues actually: for data migration & master nodes)
	require.Equal(t, (&reconciler.Results{}).WithReconciliationState(defaultRequeue.WithReason("Downscale in progress")), results)

	// voting config exclusion should have been added for leaving master
	require.True(t, esClient.AddVotingConfigExclusionsCalled)
	require.Equal(t, []string{"ssetMaster1Replicas-0"}, esClient.AddVotingConfigExclusionsCalledWith)

	// compare what has been updated in the apiserver with what we would expect
	var actual appsv1.StatefulSetList
	err := k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// simulate pods deletion that would be done by the StatefulSet controller
	require.NoError(t, k8sClient.Delete(context.Background(), &podsSsetMaster1Replica[0]))
	require.NoError(t, k8sClient.Delete(context.Background(), &podsSsetData4Replicas[3]))

	// running the downscale again should remove the next master,
	// and also requeue since data migration is still not over for data nodes
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, (&reconciler.Results{}).WithReconciliationState(defaultRequeue.WithReason("Downscale in progress")), results)
	// status should reflect the in progress operations
	require.Equal(t,
		[]esv1.DownscaledNode{
			{Name: "ssetData4Replicas-2", ShutdownStatus: "IN_PROGRESS"},
			{Name: "ssetData4Replicas-3", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster1Replicas-0", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-1", ShutdownStatus: "NOT_STARTED"},
			{Name: "ssetMaster3Replicas-2", ShutdownStatus: "COMPLETE"},
		},
		reconcileState.MergeStatusReportingWith(esv1.ElasticsearchStatus{}).DownscaleOperation.Nodes,
	)
	ssetMaster3ReplicasExpectedAfterDownscale := *ssetMaster3Replicas.DeepCopy()
	// one less master and second master sset should be gone now
	nodespec.UpdateReplicas(&ssetMaster3ReplicasExpectedAfterDownscale, pointer.Int32(2))
	expectedAfterDownscale = []appsv1.StatefulSet{ssetData4ReplicasExpectedAfterDownscale, ssetMaster3ReplicasExpectedAfterDownscale}

	err = k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// simulate master pod deletion
	require.NoError(t, k8sClient.Delete(context.Background(), &podsSsetMaster3Replicas[2]))

	// running the downscale yet again should remove the next master,
	// and also requeue since data migration is still not over for data nodes
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, (&reconciler.Results{}).WithReconciliationState(defaultRequeue.WithReason("Downscale in progress")), results)
	// status should reflect the in progress operations
	require.Equal(t,
		[]esv1.DownscaledNode{
			{Name: "ssetData4Replicas-2", ShutdownStatus: "IN_PROGRESS"},
			{Name: "ssetData4Replicas-3", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster1Replicas-0", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-1", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-2", ShutdownStatus: "COMPLETE"},
		},
		reconcileState.MergeStatusReportingWith(esv1.ElasticsearchStatus{}).DownscaleOperation.Nodes,
	)

	ssetMaster3ReplicasExpectedAfterDownscale = *ssetMaster3Replicas.DeepCopy()
	// we should be at the expected number of masters now
	nodespec.UpdateReplicas(&ssetMaster3ReplicasExpectedAfterDownscale, pointer.Int32(1))
	expectedAfterDownscale = []appsv1.StatefulSet{ssetData4ReplicasExpectedAfterDownscale, ssetMaster3ReplicasExpectedAfterDownscale}
	err = k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	require.NoError(t, k8sClient.Delete(context.Background(), &podsSsetMaster3Replicas[1]))

	// once data migration is over the downscale should continue for next data nodes
	shardLister = migration.NewFakeShardLister(
		esclient.Shards{
			{Index: "index-1", Shard: "0", State: esclient.STARTED, NodeName: "ssetData4Replicas-1"},
		},
	)
	downscaleCtx.nodeShutdown = shutdown.WithObserver(migration.NewShardMigration(es, esClient, shardLister), reconcileState)
	nodespec.UpdateReplicas(&expectedAfterDownscale[0], pointer.Int32(2))
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, emptyResults, results)
	require.Equal(t,
		[]esv1.DownscaledNode{
			{Name: "ssetData4Replicas-2", ShutdownStatus: "COMPLETE"},
			{Name: "ssetData4Replicas-3", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster1Replicas-0", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-1", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-2", ShutdownStatus: "COMPLETE"},
		},
		reconcileState.MergeStatusReportingWith(esv1.ElasticsearchStatus{}).DownscaleOperation.Nodes,
	)
	err = k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	require.Equal(t, len(expectedAfterDownscale), len(actual.Items))
	for i := range expectedAfterDownscale {
		comparison.RequireEqual(t, &expectedAfterDownscale[i], &actual.Items[i])
	}

	// data migration should have been requested for the data node leaving the cluster
	require.True(t, esClient.ExcludeFromShardAllocationCalled)
	require.Equal(t, "ssetData4Replicas-2", esClient.ExcludeFromShardAllocationCalledWith)

	// simulate pod deletion
	require.NoError(t, k8sClient.Delete(context.Background(), &podsSsetData4Replicas[2]))

	// running the downscale again should not remove any new node
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t, emptyResults, results)
	err = k8sClient.List(context.Background(), &actual)
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
	require.NoError(t, k8sClient.Create(context.Background(), &ssetToRemove))
	// do the downscale, that third StatefulSet is not part of the expected ones
	err = k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	// the StatefulSet replicas should be decreased to 0, but StatefulSet should still be around
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&ssetToRemove), &ssetToRemove)
	require.NoError(t, err)
	require.Equal(t, int32(0), sset.GetReplicas(ssetToRemove))
	// run downscale again: this time the StatefulSet should be removed
	err = k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items)
	require.False(t, results.HasError())
	require.Equal(t,
		[]esv1.DownscaledNode{
			{Name: "ssetData4Replicas-2", ShutdownStatus: "COMPLETE"},
			{Name: "ssetData4Replicas-3", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster1Replicas-0", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-1", ShutdownStatus: "COMPLETE"},
			{Name: "ssetMaster3Replicas-2", ShutdownStatus: "COMPLETE"},
			{Name: "ssetToRemove-0", ShutdownStatus: "COMPLETE"},
			{Name: "ssetToRemove-1", ShutdownStatus: "COMPLETE"},
		},
		reconcileState.MergeStatusReportingWith(esv1.ElasticsearchStatus{}).DownscaleOperation.Nodes,
	)
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&ssetToRemove), &ssetToRemove)
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
			name: "delete actual statefulsets with 0 replicas when not referenced by a nodeSet",
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
		{
			name: "do not delete actual statefulsets with 0 replicas if referenced by a nodeSet",
			expectedStatefulSets: sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      esv1.StatefulSet(clusterName, "nodeset-2"),
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(1)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      esv1.StatefulSet(clusterName, "nodeset-3"),
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
			},
			actualStatefulSets: sset.StatefulSetList{
				// statefulset with 0 replicas which has no corresponding expected statefulset and is not referenced through a nodeSet: should be deleted
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      esv1.StatefulSet(clusterName, "nodeset-1"),
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
				// statefulset with 0 replicas which has a corresponding expected statefulset with 0 replica but is used by a nodeSet: should be kept
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      esv1.StatefulSet(clusterName, "nodeset-3"),
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
			},
			wantDownscales: nil, // No downscale expected
			wantDeletions: sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      esv1.StatefulSet(clusterName, "nodeset-1"),
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: pointer.Int32(0)},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDownscales, gotDeletions := calculateDownscales(downscaleState{}, tt.expectedStatefulSets, tt.actualStatefulSets, downscaleBudgetFilter)
			require.Equal(t, tt.wantDownscales, gotDownscales)
			require.Equal(t, tt.wantDeletions, gotDeletions)
		})
	}
}

func Test_calculatePerformableDownscale(t *testing.T) {
	type args struct {
		ctx       downscaleContext
		downscale ssetDownscale
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
					nodeShutdown: migration.NewShardMigration(es, &fakeESClient{}, migration.NewFakeShardLister(esclient.Shards{})),
				},
				downscale: ssetDownscale{
					initialReplicas: 3,
					targetReplicas:  2,
					finalReplicas:   2,
				},
			},
			want: ssetDownscale{
				initialReplicas: 3,
				targetReplicas:  2,
				finalReplicas:   2,
			},
		},
		{
			name: "downscale not possible: data migration not complete",
			args: args{
				ctx: downscaleContext{
					reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
					nodeShutdown: migration.NewShardMigration(es, &fakeESClient{}, migration.NewFakeShardLister(esclient.Shards{
						{
							Index:    "index-1",
							Shard:    "0",
							NodeName: "default-2",
						},
					})),
				},
				downscale: ssetDownscale{
					statefulSet:     sset.TestSset{Name: "default"}.Build(),
					initialReplicas: 3,
					targetReplicas:  2,
					finalReplicas:   2,
				},
			},
			want: ssetDownscale{
				statefulSet:     sset.TestSset{Name: "default"}.Build(),
				initialReplicas: 3,
				targetReplicas:  3,
				finalReplicas:   2,
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
		name                    string
		downscale               ssetDownscale
		state                   *downscaleState
		statefulSets            sset.StatefulSetList
		expectedStatefulSets    []appsv1.StatefulSet
		expectedDownscaledNodes []esv1.DownscaledNode
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
			expectedDownscaledNodes: []esv1.DownscaledNode{{Name: "default-2", ShutdownStatus: "COMPLETE"}},
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
			expectedDownscaledNodes: []esv1.DownscaledNode{}, // expectedDownscaledNodes is not updated
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runtimeObjs []runtime.Object
			for i := range tt.statefulSets {
				runtimeObjs = append(runtimeObjs, &tt.statefulSets[i])
			}
			k8sClient := k8s.NewFakeClient(runtimeObjs...)
			esState := reconcile.MustNewState(esv1.Elasticsearch{})
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient),
				reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
				nodeShutdown:   shutdown.WithObserver(migration.NewShardMigration(es, &fakeESClient{}, migration.NewFakeShardLister(esclient.Shards{})), esState),
				esClient:       &fakeESClient{},
			}
			// do the downscale
			_, err := attemptDownscale(downscaleCtx, tt.downscale, tt.statefulSets)
			require.NoError(t, err)
			// retrieve statefulsets
			var ssets appsv1.StatefulSetList
			err = k8sClient.List(context.Background(), &ssets)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedStatefulSets), len(ssets.Items))
			for i := range tt.expectedStatefulSets {
				comparison.AssertEqual(t, &tt.expectedStatefulSets[i], &ssets.Items[i])
			}
			// check downscale status
			downscaleStatus := esState.DownscaleReporter.Merge(esv1.DownscaleOperation{})
			assert.Equal(t, tt.expectedDownscaledNodes, downscaleStatus.Nodes)
		})
	}
}

func Test_doDownscale_updateReplicasAndExpectations(t *testing.T) {
	sset1 := ssetMaster3Replicas
	sset1.Generation = 1
	sset2 := ssetData4Replicas
	sset2.Generation = 1
	k8sClient := k8s.NewFakeClient(&sset1, &sset2)
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
	err = k8sClient.List(context.Background(), &ssets)
	require.NoError(t, err)
	expectedSsets := []appsv1.StatefulSet{sset2, expectedSset1}
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
			k8sClient := k8s.NewFakeClient(es.DeepCopy(), &ssetMasters, &ssetData, &v7Pod)
			esClient := &fakeESClient{}
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient),
				reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
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
	controllerscheme.SetupScheme()
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
			k8sClient := k8s.NewFakeClient(tt.apiserverResources...)
			esClient := &fakeESClient{}
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient),
				reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
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
	svc := nodespec.HeadlessService(&es, sset.Name)

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
			k8sClient := k8s.NewFakeClient(tt.resources...)
			err := deleteStatefulSetResources(k8sClient, es, sset)
			require.NoError(t, err)
			// sset, cfg and headless services should not be there anymore
			require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&sset), &sset)))
			require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&cfg), &cfg)))
			require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&svc), &svc)))
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
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
			},
			wantRemaining: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name, ResourceVersion: "999"}.Build(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "two StatefulSets to delete",
			toDelete: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.Build(),
				sset.TestSset{Namespace: "ns", Name: "sset3", ClusterName: es.Name}.Build(),
			},
			objs: []runtime.Object{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset3", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
			},
			wantRemaining: sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.Build(),
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
			client := k8s.NewFakeClient(tt.objs...)
			err := deleteStatefulSets(tt.toDelete, client, es)
			if tt.wantErr != nil {
				require.True(t, tt.wantErr(err))
			} else {
				require.NoError(t, err)
			}
			var remainingSsets appsv1.StatefulSetList
			require.NoError(t, client.List(context.Background(), &remainingSsets))
			require.Equal(t, tt.wantRemaining, sset.StatefulSetList(remainingSsets.Items))
		})
	}
}
