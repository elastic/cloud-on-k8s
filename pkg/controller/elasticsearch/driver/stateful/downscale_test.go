// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Sample StatefulSets to use in tests
var (
	clusterName = "cluster-name"
	es          = esv1.Elasticsearch{
		Name:      clusterName,
		Namespace: "ns",
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
	runtimeObjs = []client.Object{&es, &ssetMaster1Replica, &ssetMaster3Replicas, &ssetData4Replicas,
		&podsSsetMaster1Replica[0], &podsSsetMaster3Replicas[0], &podsSsetMaster3Replicas[1], &podsSsetMaster3Replicas[2],
		&podsSsetData4Replicas[0], &podsSsetData4Replicas[1], &podsSsetData4Replicas[2], &podsSsetData4Replicas[3],
	}
	emptyResults = &reconciler.Results{}
)

// testLeavingNodes returns the leavingNodes of the downscale from the given actual StatefulSets to the given expected
// ones, with the shutdown interface of the given downscale context and the given ESState.
func testLeavingNodes(downscaleCtx downscaleContext, esState ESState, expected, actual es_sset.StatefulSetList) *leavingNodes {
	return newLeavingNodes(desiredLeavingNodeNames(context.Background(), expected, actual), esState, downscaleCtx.nodeShutdown, esv1.DownscaleOperation{})
}

// -- Tests start here

func TestHandleDownscale(t *testing.T) {
	// This test focuses on one code path that visits most functions.
	// Derived paths are individually tested in unit tests of the other functions.

	// We want to downscale 3 StatefulSets (masters 1-> 0, masters 3 -> 1 and data 4 -> 2) in version 7.X,
	// but should only be allowed a partial downscale (1->0, 3 -> 3 and 4 -> 3).

	k8sClient := k8s.NewFakeClient(runtimeObjs...)
	esClient := &fakeESClient{}
	actualStatefulSets := es_sset.StatefulSetList{ssetMaster1Replica, ssetMaster3Replicas, ssetData4Replicas}
	shardLister := migration.NewFakeShardLister(
		esclient.Shards{
			{Index: "index-1", Shard: "0", State: esclient.STARTED, NodeName: "ssetData4Replicas-2"},
		},
	)
	reconcileState := reconcile.MustNewState(esv1.Elasticsearch{})
	downscaleCtx := downscaleContext{
		k8sClient:      k8sClient,
		expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
		reconcileState: reconcileState,
		nodeShutdown:   shutdown.WithObserver(migration.NewShardMigration(es, esClient, shardLister), reconcileState),
		esClient:       esClient,
		es:             es,
		parentCtx:      context.Background(),
	}

	// request master nodes downscale from 3 to 1 replicas the other master StatefulSet should not be there anymore
	ssetMaster3ReplicasDownscaled := *ssetMaster3Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetMaster3ReplicasDownscaled, new(int32(1)))
	// request data nodes downscale from 4 to 2 replicas
	ssetData4ReplicasDownscaled := *ssetData4Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetData4ReplicasDownscaled, new(int32(2)))
	requestedStatefulSets := es_sset.StatefulSetList{ssetMaster3ReplicasDownscaled, ssetData4ReplicasDownscaled}

	// do the downscale
	results := HandleDownscale(downscaleCtx, requestedStatefulSets, actualStatefulSets, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actualStatefulSets))
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
	nodespec.UpdateReplicas(&ssetMaster1ReplicaExpectedAfterDownscale, new(int32(0)))

	// only part of the expected replicas of ssetData4Replicas should be updated,
	// since a node still needs to migrate data
	ssetData4ReplicasExpectedAfterDownscale := *ssetData4Replicas.DeepCopy()
	nodespec.UpdateReplicas(&ssetData4ReplicasExpectedAfterDownscale, new(int32(3)))

	expectedAfterDownscale := []appsv1.StatefulSet{ssetData4ReplicasExpectedAfterDownscale, ssetMaster1ReplicaExpectedAfterDownscale, ssetMaster3Replicas}

	// a requeue should be requested since all nodes were not downscaled
	// (2 requeues actually: for data migration & master nodes)
	require.Equal(t, (&reconciler.Results{}).WithReconciliationState(shared.DefaultRequeue.WithReason(
		"Downscale in progress, delayed nodes: [ssetMaster3Replicas-1 ssetMaster3Replicas-2], ssetMaster3Replicas: "+OneMasterAtATimeInvariant,
	)), results)

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
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actual.Items))
	require.False(t, results.HasError())
	require.Equal(t, (&reconciler.Results{}).WithReconciliationState(shared.DefaultRequeue.WithReason(
		"Downscale in progress, delayed nodes: [ssetMaster3Replicas-1], ssetMaster3Replicas: "+OneMasterAtATimeInvariant,
	)), results)
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
	nodespec.UpdateReplicas(&ssetMaster3ReplicasExpectedAfterDownscale, new(int32(2)))
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
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actual.Items))
	require.False(t, results.HasError())
	require.Equal(t, (&reconciler.Results{}).WithReconciliationState(shared.DefaultRequeue.WithReason("Downscale in progress")), results)
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
	nodespec.UpdateReplicas(&ssetMaster3ReplicasExpectedAfterDownscale, new(int32(1)))
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
	nodespec.UpdateReplicas(&expectedAfterDownscale[0], new(int32(2)))
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actual.Items))
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
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actual.Items))
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
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actual.Items))
	require.False(t, results.HasError())
	// the StatefulSet replicas should be decreased to 0, but StatefulSet should still be around
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&ssetToRemove), &ssetToRemove)
	require.NoError(t, err)
	require.Equal(t, int32(0), sset.GetReplicas(ssetToRemove))
	// run downscale again: this time the StatefulSet should be removed
	err = k8sClient.List(context.Background(), &actual)
	require.NoError(t, err)
	results = HandleDownscale(downscaleCtx, requestedStatefulSets, actual.Items, testLeavingNodes(downscaleCtx, nil, requestedStatefulSets, actual.Items))
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

// countingNodeShutdown counts the calls to ShutdownStatus for each node, and the calls to NodesWithShutdown.
type countingNodeShutdown struct {
	shutdown.Interface
	statusCalls            map[string]int
	nodesWithShutdownCalls int
}

func (c *countingNodeShutdown) NodesWithShutdown(ctx context.Context, podNames []string) (sets.Set[string], error) {
	c.nodesWithShutdownCalls++
	return c.Interface.NodesWithShutdown(ctx, podNames)
}

func (c *countingNodeShutdown) ShutdownStatus(ctx context.Context, podName string) (shutdown.NodeShutdownStatus, error) {
	if c.statusCalls == nil {
		c.statusCalls = map[string]int{}
	}
	c.statusCalls[podName]++
	return c.Interface.ShutdownStatus(ctx, podName)
}

func TestHandleDownscale_drainingNodesKeepTheirShutdown(t *testing.T) {
	type nodeSet struct {
		name            string
		master, data    bool
		actualReplicas  int32
		desiredReplicas int32
	}
	// The "old" NodeSet is being removed while the Pod of the "unhealthy" NodeSet cannot become ready. Elasticsearch
	// closes the readiness port of old-0 because of its remove-type shutdown, so only master-0 and new-0 are ready:
	// with 3 desired nodes and maxUnavailable 1, old-0 can only be removed if its removal does not consume the budget.
	removedNodeSet := []nodeSet{
		{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
		{name: "new", data: true, actualReplicas: 1, desiredReplicas: 1},
		{name: "unhealthy", data: true, actualReplicas: 1, desiredReplicas: 1},
		{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
	}

	tests := []struct {
		name           string
		nodeSets       []nodeSet
		maxUnavailable *int32
		notReadyPods   sets.Set[string]
		// notJoinedPods are the Pods that are not members of the cluster
		notJoinedPods sets.Set[string]
		// pendingPods are the Pods that are pending because they cannot be scheduled
		pendingPods sets.Set[string]
		// gatedPods are the Pods that are pending because a scheduling gate holds them
		gatedPods sets.Set[string]
		// startingPods are the Pods that are pending while they start, for example running their init containers
		startingPods sets.Set[string]
		// missingPods are the Pods of the StatefulSets that do not exist, for example because their creation is rejected
		missingPods sets.Set[string]
		// nodesChanged is true if the health counts one more node than the nodes response, as a node joined in between
		nodesChanged bool
		// partialNodes is true if the nodes response is partial: the Pods that are not members did not respond
		partialNodes bool
		// health is the health of the cluster, which is not green if left empty
		health esclient.Health
		// healthErr is returned when the health of the cluster is retrieved
		healthErr error
		// wantVotingExclusions are the nodes added to the voting config exclusions, if not nil
		wantVotingExclusions []string
		// wantRemovedEvents are the texts that the events reporting the removals of nodes that are not members of the
		// cluster must contain, in order, one per event. No such event is expected if empty.
		wantRemovedEvents []string
		// wantRemovedWarning is true if the event reporting the removal is a warning, it is a normal one otherwise
		wantRemovedWarning bool
		// deleteErr is returned when a resource is deleted, for example a StatefulSet that is no longer expected
		deleteErr error
		// updateErr is returned when a resource is updated, for example a downscaled StatefulSet
		updateErr error
		// wantStatusCalls overrides the number of times the shutdown status of a node is retrieved, for the cases that
		// stop before the shutdowns are requested
		wantStatusCalls map[string]int
		shutdowns       map[string]esclient.NodeShutdown
		// reportedShutdowns are the shutdown statuses in the downscale status reported by a previous reconciliation
		reportedShutdowns map[string]string
		// reportedExplanations are the explanations of the reported shutdown statuses, by node name
		reportedExplanations map[string]string
		getShutdownErr       error
		// putShutdownFailures are the nodes whose shutdown request fails
		putShutdownFailures sets.Set[string]
		// deleteShutdownErr is returned when a shutdown is cancelled
		deleteShutdownErr error
		wantErr           bool
		// wantErrOnce is an error text that must appear exactly once in the reported errors
		wantErrOnce           string
		wantShutdownCancelled bool
		// wantShutdowns are the node IDs of the shutdowns that remain registered, if not nil
		wantShutdowns        []string
		wantPutShutdowns     []string
		wantReplicas         map[string]int32
		wantShutdownStatuses map[string]string
		wantStalled          bool
		// wantStalledMessage is the explanation that the stalled event must contain, and wantNotStalledMessage the one it must
		// not contain
		wantStalledMessage    string
		wantNotStalledMessage string
		wantReason            string
	}{
		{
			// the next steps may recreate the Pod, for it to join the cluster and be removed
			name: "a leaving node that is not a member of the cluster is held, the downscale does not fail",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterInvariant + ": data-2",
		},
		{
			// the NodeSet is no longer expected, so the rolling upgrade cannot repair its Pod: it is removed without a
			// shutdown, even with maxUnavailable 0, as it is already unavailable
			name: "a node of a removed NodeSet that is not a member of the cluster is removed when the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			maxUnavailable: new(int32(0)),
			notReadyPods:   sets.New("old-0"),
			notJoinedPods:  sets.New("old-0"),
			health:         esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:   map[string]int32{"old": 0},
			// the node has no shutdown: it keeps the status that recording the nodes to remove resets, until the next
			// reconciliation, in which it is no longer a node to remove
			wantShutdownStatuses: map[string]string{"old-0": "NOT_STARTED"},
			wantRemovedEvents:    []string{"Removed node old-0 without a shutdown"},
		},
		{
			// the removal is only reported once done: a failed attempt is retried, and would report it again
			name: "the removal of a node of a removed NodeSet that is not a member of the cluster is not reported if it fails",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0"),
			notJoinedPods: sets.New("old-0"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			updateErr:     errors.New("update failed"),
			wantErr:       true,
			wantReplicas:  map[string]int32{"old": 1},
			// the node has no shutdown: it keeps the status that recording the nodes to remove resets
			wantShutdownStatuses: map[string]string{"old-0": "NOT_STARTED"},
		},
		{
			// the nodes are removed by highest ordinal first: old-0 waits for old-1, which is still draining
			name: "a node of a removed NodeSet that is not a member of the cluster waits for the draining node with a higher ordinal",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0", "old-1"),
			notJoinedPods: sets.New("old-0"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantReplicas:         map[string]int32{"old": 2},
			wantShutdownStatuses: map[string]string{"old-1": "IN_PROGRESS"},
			wantReason:           "Downscale in progress",
		},
		{
			name: "a node of a removed NodeSet that is not a member of the cluster is removed after the draining node with a higher ordinal",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0", "old-1"),
			notJoinedPods: sets.New("old-0"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantReplicas:         map[string]int32{"old": 0},
			wantShutdownStatuses: map[string]string{"old-1": "COMPLETE"},
			wantRemovedEvents:    []string{"Removed node old-0 without a shutdown"},
		},
		{
			name: "a node of a removed NodeSet that is not a member of the cluster is held when the cluster is not green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0"),
			notJoinedPods: sets.New("old-0"),
			health:        esclient.Health{Status: esv1.ElasticsearchYellowHealth},
			wantReplicas:  map[string]int32{"old": 1},
			wantReason:    "Downscale in progress, delayed nodes: [old-0], old: " + NodeNotInClusterNotSettledInvariant + ": old-0",
		},
		{
			name: "a node of a removed NodeSet that is not a member of the cluster is held while shards are relocating",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0"),
			notJoinedPods: sets.New("old-0"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth, RelocatingShards: 1},
			wantReplicas:  map[string]int32{"old": 1},
			wantReason:    "Downscale in progress, delayed nodes: [old-0], old: " + NodeNotInClusterNotSettledInvariant + ": old-0",
		},
		{
			// the health cannot be retrieved: the cluster is not known to be settled, so old-0 is held, and the rest of the
			// downscale carries on, data-2 is removed once its shutdown is complete
			name: "a node of a removed NodeSet that is not a member of the cluster is held if the health cannot be retrieved, the downscale carries on",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("old-0"),
			notJoinedPods: sets.New("old-0"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			healthErr:     errors.New("health failed"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
			},
			wantReplicas:         map[string]int32{"old": 1, "data": 2},
			wantShutdownStatuses: map[string]string{"data-2": "COMPLETE"},
			wantReason:           "Downscale in progress, delayed nodes: [old-0], old: " + NodeNotInClusterNotSettledInvariant + ": old-0",
		},
		{
			// the node may be a member that did not answer: it must not be removed, whatever the health of the cluster
			name: "a node of a removed NodeSet that is not a member of the cluster is held if the nodes response is partial",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0"),
			notJoinedPods: sets.New("old-0"),
			partialNodes:  true,
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:  map[string]int32{"old": 1},
			wantReason:    "Downscale in progress, delayed nodes: [old-0], old: " + NodeNotInClusterPartialInvariant + ": old-0",
		},
		{
			// the NodeSet was scaled up beyond the capacity, then back down: the Pod cannot be scheduled
			name: "a pending node of a downscaled NodeSet is removed when the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:      sets.New("data-2"),
			notJoinedPods:     sets.New("data-2"),
			pendingPods:       sets.New("data-2"),
			health:            esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:      map[string]int32{"data": 2},
			wantRemovedEvents: []string{"Removed node data-2 without a shutdown"},
		},
		{
			// a scheduling gate holds the Pod
			name: "a gated node of a downscaled NodeSet is removed when the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:      sets.New("data-2"),
			notJoinedPods:     sets.New("data-2"),
			gatedPods:         sets.New("data-2"),
			health:            esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:      map[string]int32{"data": 2},
			wantRemovedEvents: []string{"Removed node data-2 without a shutdown"},
		},
		{
			// data-2 was a member, then the rolling upgrade restarted it for a spec that cannot be scheduled: its restart
			// shutdown is left, the downscale removes it without waiting for it, and leaves the shutdown to the upgrade
			name: "a pending node of a downscaled NodeSet that left the cluster for an upgrade is removed when the cluster is green, its restart shutdown is kept",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			pendingPods:   sets.New("data-2"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Restart), Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantShutdowns:         []string{"id-data-2"},
			wantReplicas:          map[string]int32{"data": 2},
			wantRemovedEvents:     []string{"Removed node data-2 without a shutdown"},
		},
		{
			// the creation of the Pod is rejected, for example by a resource quota: nothing can make it join the cluster
			name: "a missing node of a downscaled NodeSet is removed when the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notJoinedPods:     sets.New("data-2"),
			missingPods:       sets.New("data-2"),
			health:            esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:      map[string]int32{"data": 2},
			wantRemovedEvents: []string{"Removed node data-2 without a shutdown"},
		},
		{
			// the node may be a member that did not answer, whether its Pod exists or not
			name: "a missing node of a downscaled NodeSet is held if the nodes response is partial",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notJoinedPods: sets.New("data-2"),
			missingPods:   sets.New("data-2"),
			partialNodes:  true,
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterPartialInvariant + ": data-2",
		},
		{
			// a previous Pod of the same name may have been a member, and its storage may hold data
			name: "a pending node of a downscaled NodeSet is held when the cluster is not green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			pendingPods:   sets.New("data-2"),
			health:        esclient.Health{Status: esv1.ElasticsearchYellowHealth},
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterNotSettledInvariant + ": data-2",
		},
		{
			// the creation of the Pod is rejected, for example by a resource quota: a previous Pod of the same name may have been
			// a member, and its storage may hold data
			name: "a missing node of a downscaled NodeSet is held when the cluster is not green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notJoinedPods: sets.New("data-2"),
			missingPods:   sets.New("data-2"),
			health:        esclient.Health{Status: esv1.ElasticsearchYellowHealth},
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterNotSettledInvariant + ": data-2",
		},
		{
			// as in the issue this covers: the Pod of a removed NodeSet was never scheduled
			name: "a pending node of a removed NodeSet is removed when the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:      sets.New("old-0"),
			notJoinedPods:     sets.New("old-0"),
			pendingPods:       sets.New("old-0"),
			health:            esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:      map[string]int32{"old": 0},
			wantRemovedEvents: []string{"Removed node old-0 without a shutdown: it is not a member of the cluster and cannot join it, and the cluster is green with no shard activity: its storage holds no shard copy that the cluster lacks"},
		},
		{
			// a node joined between the nodes request and the health one, it may be the one that is not a member, whose
			// shards the green health may account for
			name: "a node of a removed NodeSet that is not a member of the cluster is held if the nodes changed before the health is retrieved",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:  sets.New("old-0"),
			notJoinedPods: sets.New("old-0"),
			nodesChanged:  true,
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:  map[string]int32{"old": 1},
			wantReason:    "Downscale in progress, delayed nodes: [old-0], old: " + NodeNotInClusterNotSettledInvariant + ": old-0",
		},
		{
			// data-2 cannot be scheduled and data-1 does not exist: neither can join the cluster, both are removed
			name: "a pending node of a downscaled NodeSet is removed when the cluster is green, with a missing node after it",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			notReadyPods:      sets.New("data-2"),
			notJoinedPods:     sets.New("data-1", "data-2"),
			pendingPods:       sets.New("data-2"),
			missingPods:       sets.New("data-1"),
			health:            esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:      map[string]int32{"data": 1},
			wantRemovedEvents: []string{"Removed node data-2 without a shutdown", "Removed node data-1 without a shutdown"},
		},
		{
			// the Pod is scheduled and pending while it starts, for example running its init containers: it may join
			name: "a starting node of a downscaled NodeSet that is not a member of the cluster is held, even if the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			startingPods:  sets.New("data-2"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterInvariant + ": data-2",
		},
		{
			// a running Pod may be a member that restarts: it is not removed without a shutdown
			name: "a running node of a downscaled NodeSet that is not a member of the cluster is held, even if the cluster is green",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			health:        esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterInvariant + ": data-2",
		},
		{
			// the voting configuration is told about the master by name, as it is not a node of the cluster
			name: "a master of a removed NodeSet that is not a member of the cluster is removed and excluded from the voting",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "m2", master: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods:         sets.New("m2-0"),
			notJoinedPods:        sets.New("m2-0"),
			health:               esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:         map[string]int32{"m2": 0},
			wantVotingExclusions: []string{"m2-0"},
			// a master that used to be a member may still be in the voting configuration, the message says so, in a warning
			wantRemovedEvents:  []string{"it may still be in the voting configuration"},
			wantRemovedWarning: true,
		},
		{
			// one master is removed at a time: m2-1 is removed and excluded from the voting, m2-0 is removed in the next
			// reconciliation, with its own exclusion and event
			name: "the masters of a removed NodeSet that are not members of the cluster are removed one at a time",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "m2", master: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods:         sets.New("m2-0", "m2-1"),
			notJoinedPods:        sets.New("m2-0", "m2-1"),
			health:               esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			wantReplicas:         map[string]int32{"m2": 1},
			wantVotingExclusions: []string{"m2-1"},
			wantRemovedEvents:    []string{"Removed node m2-1 without a shutdown"},
			wantRemovedWarning:   true,
			wantReason:           "Downscale in progress, delayed nodes: [m2-0], m2: " + OneMasterAtATimeInvariant,
		},
		{
			// the nodes response is partial: data-2 may be a member that did not answer, the reason says so
			name: "a leaving node that is not a member of the cluster is held with a reason that says it may not have answered if the response is partial",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			partialNodes:  true,
			wantReplicas:  map[string]int32{"data": 3},
			wantReason:    "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterPartialInvariant + ": data-2",
		},
		{
			// data-2 left the cluster while draining, for example because its Pod restarted: its shutdown cannot be
			// mapped to it, so it is not cancelled while data-2 is held
			name: "the shutdown of a draining node that is not a member of the cluster is not cancelled",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-2"),
			notJoinedPods: sets.New("data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantShutdowns:         []string{"id-data-2"},
			wantReplicas:          map[string]int32{"data": 3},
			wantReason:            "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterInvariant + ": data-2",
		},
		{
			// data-1 no longer leaves but kept its shutdown: it is a member of the cluster, so its shutdown is cancelled,
			// while the one of data-2, which is not a member, is kept
			name: "the shutdowns of members are cancelled while a draining node is not a member of the cluster",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods:  sets.New("data-1", "data-2"),
			notJoinedPods: sets.New("data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-1": {NodeID: "id-data-1", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: true,
			wantShutdowns:         []string{"id-data-2"},
			wantReplicas:          map[string]int32{"data": 3},
			wantReason:            "Downscale in progress, delayed nodes: [data-2], data: " + NodeNotInClusterInvariant + ": data-2",
		},
		{
			// data went 3 -> 1 with an index pinned to data-2, then 1 -> 2: data-1 no longer leaves but kept its shutdown,
			// which would use up the budget for as long as data-2 is held draining
			name: "the shutdown of a node that no longer leaves is cancelled while another node is held draining",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods: sets.New("data-1", "data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-1": {NodeID: "id-data-1", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: true,
			wantShutdowns:         []string{"id-data-2"},
			wantReplicas:          map[string]int32{"data": 3},
			wantShutdownStatuses:  map[string]string{"data-2": "IN_PROGRESS"},
		},
		{
			// data-2 is a draining node that did not respond to the nodes request, reported as stalled by the previous
			// reconciliation: it keeps that status, with its phase and event, although the request of another shutdown fails
			name: "a stalled node missing from a partial nodes response is reported as stalled when a shutdown request fails",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
				{name: "other", data: true, actualReplicas: 2, desiredReplicas: 1},
			},
			maxUnavailable:       new(int32(2)),
			notJoinedPods:        sets.New("data-2"),
			partialNodes:         true,
			reportedShutdowns:    map[string]string{"data-2": "STALLED"},
			putShutdownFailures:  sets.New("id-other-1"),
			wantErr:              true,
			wantPutShutdowns:     []string{"id-other-1"},
			wantReplicas:         map[string]int32{"data": 3, "other": 2},
			wantShutdownStatuses: map[string]string{"data-2": "STALLED", "other-1": "NOT_STARTED"},
			wantStalled:          true,
		},
		{
			// the StatefulSet that is no longer expected cannot be deleted, before any shutdown is requested: the stalled
			// status of data-2, a draining member, still comes with its phase and event
			name: "the stalled status of a draining node is kept when a StatefulSet cannot be deleted",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
				{name: "old", data: true, actualReplicas: 0, desiredReplicas: 0},
			},
			notReadyPods: sets.New("data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownStalled},
			},
			deleteErr:            errors.New("delete failed"),
			wantErr:              true,
			wantReplicas:         map[string]int32{"data": 3},
			wantShutdownStatuses: map[string]string{"data-2": "STALLED"},
			wantStatusCalls:      map[string]int{"data-2": 1},
			wantStalled:          true,
		},
		{
			// the StatefulSet that is no longer expected cannot be deleted, before any shutdown is requested: the carried
			// forward stalled status of data-2 still comes with its phase and event
			name: "a stalled node missing from a partial nodes response is reported as stalled when a StatefulSet cannot be deleted",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
				{name: "old", data: true, actualReplicas: 0, desiredReplicas: 0},
			},
			notJoinedPods:        sets.New("data-2"),
			partialNodes:         true,
			reportedShutdowns:    map[string]string{"data-2": "STALLED"},
			deleteErr:            errors.New("delete failed"),
			wantErr:              true,
			wantReplicas:         map[string]int32{"data": 3},
			wantShutdownStatuses: map[string]string{"data-2": "STALLED"},
			wantStalled:          true,
		},
		{
			// data-2 did not respond and was stalled: its explanation is the one of an earlier reconciliation. It sorts before
			// other-1, which is stalled right now: the event shows the explanation of other-1
			name: "a stalled node that responds is reported rather than a stalled node missing from a partial nodes response",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
				{name: "other", data: true, actualReplicas: 2, desiredReplicas: 1},
			},
			maxUnavailable: new(int32(2)),
			notReadyPods:   sets.New("other-1"),
			notJoinedPods:  sets.New("data-2"),
			partialNodes:   true,
			shutdowns: map[string]esclient.NodeShutdown{
				"id-other-1": {
					NodeID: "id-other-1", Type: string(esclient.Remove), Status: esclient.ShutdownStalled,
					ShardMigration: esclient.ShardMigration{Explanation: "current explanation"},
				},
			},
			reportedShutdowns:     map[string]string{"data-2": "STALLED"},
			reportedExplanations:  map[string]string{"data-2": "old explanation"},
			wantReplicas:          map[string]int32{"data": 3, "other": 2},
			wantShutdownStatuses:  map[string]string{"data-2": "STALLED", "other-1": "STALLED"},
			wantStalled:           true,
			wantStalledMessage:    "current explanation",
			wantNotStalledMessage: "old explanation",
		},
		{
			// data-2 comes first in the removal order: its shutdown is confirmed, the request of data-1 fails, so only
			// data-2 is reported as shutting down
			name: "a node whose shutdown request fails is not reported as shutting down",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			maxUnavailable:       new(int32(2)),
			putShutdownFailures:  sets.New("id-data-1"),
			wantErr:              true,
			wantPutShutdowns:     []string{"id-data-2", "id-data-1"},
			wantReplicas:         map[string]int32{"data": 3},
			wantShutdownStatuses: map[string]string{"data-2": "IN_PROGRESS", "data-1": "NOT_STARTED"},
		},
		{
			// the request of data-2 fails first: data-1 is not reached, none of them is reported as shutting down
			name: "nodes whose shutdown is not reached after a failure are not reported as shutting down",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			maxUnavailable:       new(int32(2)),
			putShutdownFailures:  sets.New("id-data-2"),
			wantErr:              true,
			wantPutShutdowns:     []string{"id-data-2"},
			wantReplicas:         map[string]int32{"data": 3},
			wantShutdownStatuses: map[string]string{"data-2": "NOT_STARTED", "data-1": "NOT_STARTED"},
		},
		{
			name: "a node whose shutdown request succeeds is reported as shutting down",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			maxUnavailable:       new(int32(2)),
			wantPutShutdowns:     []string{"id-data-2", "id-data-1"},
			wantReplicas:         map[string]int32{"data": 3},
			wantShutdownStatuses: map[string]string{"data-2": "IN_PROGRESS", "data-1": "IN_PROGRESS"},
		},
		{
			// the request of the shutdown of data-1 fails after the stalled shutdown of data-2 is confirmed: its status and the
			// stalled report are kept, they are not replaced by an in progress one
			name: "a failure to request a shutdown does not hide the stalled shutdown of another node",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			maxUnavailable: new(int32(2)),
			notReadyPods:   sets.New("data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownStalled},
			},
			putShutdownFailures:  sets.New("id-data-1"),
			wantErr:              true,
			wantPutShutdowns:     []string{"id-data-1"},
			wantReplicas:         map[string]int32{"data": 3},
			wantShutdownStatuses: map[string]string{"data-2": "STALLED", "data-1": "NOT_STARTED"},
			wantStalled:          true,
		},
		{
			// the request of the shutdown of data-2 fails: data-1, which stays, still gets its shutdown cancelled
			name: "the shutdown of a node that no longer leaves is cancelled even if the shutdown request of another node fails",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			maxUnavailable: new(int32(2)),
			notReadyPods:   sets.New("data-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-1": {NodeID: "id-data-1", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
			},
			putShutdownFailures:   sets.New("id-data-2"),
			wantErr:               true,
			wantShutdownCancelled: true,
			wantShutdowns:         []string{},
			wantPutShutdowns:      []string{"id-data-2"},
			wantReplicas:          map[string]int32{"data": 3},
			wantShutdownStatuses:  map[string]string{"data-2": "NOT_STARTED"},
		},
		{
			// the cancel of the shutdown of data-1 fails: it does not hide the stalled shutdown of data-2, nor its status
			name: "a failure to cancel the shutdown of a node that no longer leaves does not hide the stalled draining node",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods: sets.New("data-1", "data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-1": {NodeID: "id-data-1", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownStalled},
			},
			deleteShutdownErr:     errors.New("delete failed"),
			wantErr:               true,
			wantShutdownCancelled: true,
			wantShutdowns:         []string{"id-data-1", "id-data-2"},
			wantReplicas:          map[string]int32{"data": 3},
			wantShutdownStatuses:  map[string]string{"data-2": "STALLED"},
			wantStalled:           true,
		},
		{
			// data-2 is drained: it is removed even though the cancel of the shutdown of data-1 fails, which is returned
			name: "a failure to cancel the shutdown of a node that no longer leaves does not hold back a downscale",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods: sets.New("data-1", "data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-1": {NodeID: "id-data-1", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
			},
			deleteShutdownErr:     errors.New("delete failed"),
			wantErr:               true,
			wantShutdownCancelled: true,
			wantShutdowns:         []string{"id-data-1", "id-data-2"},
			wantReplicas:          map[string]int32{"data": 2},
			wantShutdownStatuses:  map[string]string{"data-2": "COMPLETE"},
		},
		{
			name: "the shutdown of a node that is still leaving is kept while another node is held draining",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			notReadyPods: sets.New("data-1", "data-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-1": {NodeID: "id-data-1", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantShutdowns:         []string{"id-data-1", "id-data-2"},
		},
		{
			name: "the leaving nodes removed before a node that is not a member of the cluster are removed",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			notReadyPods:  sets.New("data-2", "data-1"),
			notJoinedPods: sets.New("data-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownComplete},
			},
			wantReplicas: map[string]int32{"data": 2},
			wantReason:   "Downscale in progress, delayed nodes: [data-1], data: " + NodeNotInClusterInvariant + ": data-1",
		},
		{
			// nothing is leaving, so the shutdowns are retrieved by the shutdown requests, and again by the cancel: the same
			// failure is reported once
			name: "a failure to retrieve the shutdowns is reported once when no node is leaving",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "data", data: true, actualReplicas: 2, desiredReplicas: 2},
			},
			getShutdownErr: errors.New("boom"),
			wantErr:        true,
			wantErrOnce:    "boom",
			wantReplicas:   map[string]int32{"data": 2},
		},
		{
			// the shutdowns are retrieved before the StatefulSets downscaled to 0 replicas are deleted
			name: "an error while retrieving the shutdowns aborts the downscale",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "removed", data: true, actualReplicas: 0, desiredReplicas: 0},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			getShutdownErr:        errors.New("boom"),
			wantErr:               true,
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"removed": 0, "old": 1},
		},
		{
			// the forced upgrade relies on the reported shutdowns while Elasticsearch is unreachable: a failed
			// retrieval must not reset them
			name:                 "an error while retrieving the shutdowns keeps the reported shutdown statuses",
			nodeSets:             removedNodeSet,
			notReadyPods:         sets.New("old-0", "unhealthy-0"),
			reportedShutdowns:    map[string]string{"old-0": "IN_PROGRESS"},
			getShutdownErr:       errors.New("boom"),
			wantErr:              true,
			wantReplicas:         map[string]int32{"old": 1},
			wantShutdownStatuses: map[string]string{"old-0": "IN_PROGRESS"},
		},
		{
			name:         "removing a draining node does not consume the budget",
			nodeSets:     removedNodeSet,
			notReadyPods: sets.New("unhealthy-0", "old-0"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 0},
		},
		{
			// the budget does not allow removing old-0, so no shutdown is requested for it and the shutdown of
			// unhealthy-0 is cancelled
			name:         "a draining node that stays does not allow removals",
			nodeSets:     removedNodeSet,
			notReadyPods: sets.New("unhealthy-0", "old-0"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-unhealthy-0": {NodeID: "id-unhealthy-0", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: true,
		},
		{
			// the draining master is not running, so removing it does not leave the cluster without a running master
			name: "a draining master node can be removed while another master is running",
			nodeSets: []nodeSet{
				{name: "mdi", master: true, data: true, actualReplicas: 2, desiredReplicas: 1},
			},
			notReadyPods: sets.New("mdi-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-mdi-1": {NodeID: "id-mdi-1", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"mdi": 1},
		},
		{
			// a-1 comes first in name order but the budget only allows removing the draining b-1, so no shutdown is
			// requested for a-1
			name: "a draining node does not allow removing a ready node of another NodeSet",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "a", data: true, actualReplicas: 2, desiredReplicas: 1},
				{name: "b", data: true, actualReplicas: 2, desiredReplicas: 1},
				{name: "c", data: true, actualReplicas: 3, desiredReplicas: 3},
			},
			notReadyPods: sets.New("b-1", "c-1", "c-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-b-1": {NodeID: "id-b-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
		},
		{
			// a-1 comes first in name order but b-1 is already draining, so no shutdown is requested for a-1
			name: "a draining master node prevents removing another master node",
			nodeSets: []nodeSet{
				{name: "a", master: true, data: true, actualReplicas: 2, desiredReplicas: 1},
				{name: "b", master: true, data: true, actualReplicas: 2, desiredReplicas: 1},
			},
			notReadyPods: sets.New("b-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-b-1": {NodeID: "id-b-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantReason:            "Downscale in progress, delayed nodes: [a-1], a: " + OneMasterAtATimeInvariant + ": b-1 draining",
		},
		{
			// a-1 is draining and comes first in name order: its removal is in progress, so no shutdown is requested for b-1
			name: "a draining master node of the first NodeSet prevents removing a master node of another NodeSet",
			nodeSets: []nodeSet{
				{name: "a", master: true, data: true, actualReplicas: 2, desiredReplicas: 1},
				{name: "b", master: true, data: true, actualReplicas: 2, desiredReplicas: 1},
			},
			notReadyPods: sets.New("a-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-a-1": {NodeID: "id-a-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantReason:            "Downscale in progress, delayed nodes: [b-1], b: " + OneMasterAtATimeInvariant,
		},
		{
			// mdi-1 has a shutdown but is not the next master to remove: it does not block the removal of mdi-2, which
			// gets a shutdown, and keeps its own shutdown
			name: "a draining master node that is not the next to be removed does not prevent removing a master node",
			nodeSets: []nodeSet{
				{name: "mdi", master: true, data: true, actualReplicas: 3, desiredReplicas: 1},
			},
			notReadyPods: sets.New("mdi-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-mdi-1": {NodeID: "id-mdi-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantPutShutdowns:      []string{"id-mdi-2"},
			wantReplicas:          map[string]int32{"mdi": 3},
			wantShutdownStatuses:  map[string]string{"mdi-1": "IN_PROGRESS", "mdi-2": "IN_PROGRESS"},
		},
		{
			// mdi-2 is drained but mdi-1 is not ready: with 2 desired nodes and maxUnavailable 1 the budget does not allow
			// removing mdi-2, which keeps its shutdown
			name: "a draining master node is not removed while the budget does not allow it",
			nodeSets: []nodeSet{
				{name: "mdi", master: true, data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			notReadyPods: sets.New("mdi-1", "mdi-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-mdi-2": {NodeID: "id-mdi-2", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"mdi": 3},
			wantShutdownStatuses:  map[string]string{"mdi-2": "COMPLETE"},
		},
		{
			// mdi-2 is drained and the other masters are ready: removing it does not consume the budget
			name: "a draining master node is removed with maxUnavailable 0 while the other masters are ready",
			nodeSets: []nodeSet{
				{name: "mdi", master: true, data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			maxUnavailable: new(int32(0)),
			notReadyPods:   sets.New("mdi-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-mdi-2": {NodeID: "id-mdi-2", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"mdi": 2},
		},
		{
			// mdi-2 is drained and the other masters are ready: with 2 ready nodes, 2 desired nodes and maxUnavailable 1,
			// the budget allows removing mdi-2
			name: "a draining master node is removed with maxUnavailable 1 while the other nodes are ready",
			nodeSets: []nodeSet{
				{name: "mdi", master: true, data: true, actualReplicas: 3, desiredReplicas: 2},
			},
			maxUnavailable: new(int32(1)),
			notReadyPods:   sets.New("mdi-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-mdi-2": {NodeID: "id-mdi-2", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"mdi": 2},
		},
		{
			// with maxSurge 0, the new NodeSet cannot be created before the old one is removed: with 2 ready nodes,
			// 3 desired nodes and maxUnavailable 1 the budget does not allow removing old-2, but the other masters are ready
			name: "a draining master node of a renamed NodeSet is removed with maxSurge 0",
			nodeSets: []nodeSet{
				{name: "new", master: true, data: true, actualReplicas: 0, desiredReplicas: 3},
				{name: "old", master: true, data: true, actualReplicas: 3, desiredReplicas: 0},
			},
			maxUnavailable: new(int32(1)),
			notReadyPods:   sets.New("old-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-2": {NodeID: "id-old-2", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
		},
		{
			// new-2 cannot be scheduled before old-2 is removed: with 4 ready nodes, 3 desired nodes and maxUnavailable 1,
			// the budget allows removing old-2
			name: "a draining master node is removed while a new master node is not ready if the budget allows it",
			nodeSets: []nodeSet{
				{name: "new", master: true, data: true, actualReplicas: 3, desiredReplicas: 3},
				{name: "old", master: true, data: true, actualReplicas: 3, desiredReplicas: 0},
			},
			notReadyPods: sets.New("new-2", "old-2"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-2": {NodeID: "id-old-2", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
		},
		{
			// old-0 is drained but the new NodeSet has no Pod yet: old-0 is not removed and keeps its shutdown
			name: "a draining master node is not removed while no other master node is running",
			nodeSets: []nodeSet{
				{name: "new", master: true, data: true, actualReplicas: 0, desiredReplicas: 1},
				{name: "old", master: true, data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods: sets.New("old-0"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 1},
		},
		{
			// both shutdowns are complete but only old-1 can be removed with maxUnavailable 1, old-0 keeps its shutdown
			// as it is draining
			name: "draining nodes beyond the draining removals allowed keep their shutdown",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "unhealthy", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "old-0", "old-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownComplete},
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 1},
			wantShutdownStatuses:  map[string]string{"old-0": "COMPLETE", "old-1": "COMPLETE"},
		},
		{
			// old-1 is selected but not drained yet, old-0 is held: the reason mentions it even though the downscale of
			// old-1 is in progress
			name: "the delayed nodes are reported while the downscale of other nodes is in progress",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "unhealthy", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "old-0", "old-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownComplete},
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
			wantShutdownStatuses:  map[string]string{"old-0": "COMPLETE", "old-1": "IN_PROGRESS"},
			wantReason:            "Downscale in progress, delayed nodes: [old-0], old: " + RespectMaxUnavailableInvariant,
		},
		{
			// old-0 stays ready while it drains, as with the legacy readiness probe, so removing it needs budget, which the
			// 2 unhealthy Pods use up: it is not removed but keeps its shutdown
			name: "a ready draining node keeps its shutdown when the budget does not allow its removal",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "unhealthy", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "unhealthy-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownComplete},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 1},
			wantShutdownStatuses:  map[string]string{"old-0": "COMPLETE"},
			wantReason:            "Downscale in progress, delayed nodes: [old-0], old: " + RespectMaxUnavailableInvariant,
		},
		{
			name: "a stalled shutdown of a draining node that is not removed is reported",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "unhealthy", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "old", data: true, actualReplicas: 1, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "unhealthy-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownStalled},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 1},
			wantShutdownStatuses:  map[string]string{"old-0": "STALLED"},
			wantStalled:           true,
		},
		{
			// old-1 is selected but not drained yet, old-0 is held: the phase reports the stalled shutdown of old-0 rather
			// than the data migration of old-1
			name: "a stalled shutdown of a draining node that is not removed takes precedence over data migration",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "unhealthy", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "old-0", "old-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownStalled},
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
			wantShutdownStatuses:  map[string]string{"old-0": "STALLED", "old-1": "IN_PROGRESS"},
			wantStalled:           true,
		},
		{
			// old-1 is selected and old-0 is held, both shutdowns are stalled: a single stalled event is reported for both
			name: "the stalled shutdowns of the nodes that are removed and held are reported once",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "unhealthy", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "old-0", "old-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownStalled},
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownStalled},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
			wantShutdownStatuses:  map[string]string{"old-0": "STALLED", "old-1": "STALLED"},
			wantStalled:           true,
		},
		{
			// old-1 is selected but its shutdown could not be started, which fails the downscale: the stalled shutdown of
			// the held old-0 is still reported
			name: "a stalled shutdown is reported when a downscale fails",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "unhealthy", data: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			notReadyPods: sets.New("unhealthy-0", "old-0", "old-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownStalled},
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownNotStarted},
			},
			wantErr:               true,
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
			wantShutdownStatuses:  map[string]string{"old-0": "STALLED", "old-1": "NOT_STARTED"},
			wantStalled:           true,
		},
		{
			// both nodes are selected but old-0 waits for old-1 to be drained: the status of old-0 is still reported
			name: "the shutdown status of a node removed after a node that is not drained is reported",
			nodeSets: []nodeSet{
				{name: "master", master: true, actualReplicas: 1, desiredReplicas: 1},
				{name: "new", data: true, actualReplicas: 2, desiredReplicas: 2},
				{name: "old", data: true, actualReplicas: 2, desiredReplicas: 0},
			},
			maxUnavailable: new(int32(2)),
			notReadyPods:   sets.New("old-0", "old-1"),
			shutdowns: map[string]esclient.NodeShutdown{
				"id-old-0": {NodeID: "id-old-0", Type: "REMOVE", Status: esclient.ShutdownComplete},
				"id-old-1": {NodeID: "id-old-1", Type: "REMOVE", Status: esclient.ShutdownInProgress},
			},
			wantShutdownCancelled: false,
			wantReplicas:          map[string]int32{"old": 2},
			wantShutdownStatuses:  map[string]string{"old-0": "COMPLETE", "old-1": "IN_PROGRESS"},
			wantReason:            "Downscale in progress",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{Namespace: "ns", Name: clusterName}
			es.Spec.UpdateStrategy.ChangeBudget.MaxUnavailable = tt.maxUnavailable
			for name, status := range tt.reportedShutdowns {
				// the status was written by a previous reconciliation
				es.Status.DownscaleOperation.LastUpdatedTime = metav1.Now()
				reported := esv1.DownscaledNode{Name: name, ShutdownStatus: status}
				if explanation, exists := tt.reportedExplanations[name]; exists {
					reported.Explanation = &explanation
				}
				es.Status.DownscaleOperation.Nodes = append(es.Status.DownscaleOperation.Nodes, reported)
			}
			var actualStatefulSets, expectedStatefulSets es_sset.StatefulSetList
			var pods []corev1.Pod
			for _, ns := range tt.nodeSets {
				testSset := sset.TestSset{Namespace: es.Namespace, Name: ns.name, ClusterName: es.Name, Master: ns.master, Data: ns.data}
				testSset.Replicas = ns.actualReplicas
				actualStatefulSet := testSset.Build()
				actualStatefulSets = append(actualStatefulSets, actualStatefulSet)
				if ns.desiredReplicas > 0 {
					es.Spec.NodeSets = append(es.Spec.NodeSets, esv1.NodeSet{Name: ns.name, Count: ns.desiredReplicas})
					testSset.Replicas = ns.desiredReplicas
					expectedStatefulSets = append(expectedStatefulSets, testSset.Build())
				}
				for i := range ns.actualReplicas {
					name := sset.PodName(ns.name, i)
					if tt.missingPods.Has(name) {
						continue
					}
					var phase corev1.PodPhase
					if tt.pendingPods.Has(name) || tt.gatedPods.Has(name) || tt.startingPods.Has(name) {
						phase = corev1.PodPending
					}
					pod := sset.TestPod{Namespace: es.Namespace, Name: name, ClusterName: es.Name, StatefulSetName: ns.name, Master: ns.master, Data: ns.data, Ready: !tt.notReadyPods.Has(name), Phase: phase}.Build()
					switch {
					case tt.pendingPods.Has(name):
						pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable})
					case tt.gatedPods.Has(name):
						pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonSchedulingGated})
					case tt.startingPods.Has(name):
						pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue})
					}
					pods = append(pods, pod)
				}
			}

			objs := make([]client.Object, 0, 1+len(actualStatefulSets)+len(pods))
			objs = append(objs, &es)
			for _, s := range actualStatefulSets {
				objs = append(objs, s.DeepCopy())
			}
			podToNodeID := map[string]string{}
			nodes := esclient.Nodes{Nodes: map[string]esclient.Node{}}
			for _, p := range pods {
				objs = append(objs, p.DeepCopy())
				if tt.notJoinedPods.Has(p.Name) {
					continue
				}
				podToNodeID[p.Name] = "id-" + p.Name
				nodes.Nodes["id-"+p.Name] = esclient.Node{Name: p.Name}
			}
			if tt.partialNodes {
				nodes.Summary.Failed = len(tt.notJoinedPods)
			}
			// the health counts the nodes of the nodes response, unless a node joined in between
			health := tt.health
			health.NumberOfNodes = len(nodes.Nodes)
			if tt.nodesChanged {
				health.NumberOfNodes++
			}
			clientBuilder := k8s.NewFakeClientBuilder(objs...)
			if tt.deleteErr != nil || tt.updateErr != nil {
				clientBuilder = clientBuilder.WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if tt.deleteErr != nil {
							return tt.deleteErr
						}
						return c.Delete(ctx, obj, opts...)
					},
					Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						if tt.updateErr != nil {
							return tt.updateErr
						}
						return c.Update(ctx, obj, opts...)
					},
				})
			}
			k8sClient := clientBuilder.Build()
			esClient := &fakeESClient{
				version: version.MustParse("8.0.0"), nodes: nodes, health: health, GetClusterHealthErr: tt.healthErr, Shutdowns: maps.Clone(tt.shutdowns), GetShutdownErr: tt.getShutdownErr,
				RegisterPutShutdowns: true, PutShutdownErrNodes: tt.putShutdownFailures, DeleteShutdownErr: tt.deleteShutdownErr,
			}
			reconcileState := reconcile.MustNewState(es)
			nodeShutdown := &countingNodeShutdown{
				Interface: shutdown.WithObserver(
					shutdown.NewNodeShutdown(esClient, podToNodeID, esclient.Remove, "", nil, logr.Discard()),
					reconcileState.DownscaleReporter,
				),
			}
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				reconcileState: reconcileState,
				nodeShutdown:   nodeShutdown,
				esClient:       esClient,
				es:             es,
				parentCtx:      context.Background(),
			}

			results := HandleDownscale(downscaleCtx, expectedStatefulSets, actualStatefulSets, leavingNodesOfStatus(es, expectedStatefulSets, actualStatefulSets, esClient, downscaleCtx.nodeShutdown))
			// the shutdowns of the draining nodes are already registered, only the nodes selected for removal get one
			var putShutdowns []string
			for _, call := range esClient.PutShutdownCalls {
				putShutdowns = append(putShutdowns, call.NodeID)
			}
			require.Equal(t, tt.wantPutShutdowns, putShutdowns)
			require.Equal(t, tt.wantErr, results.HasError())
			if tt.wantVotingExclusions != nil {
				require.Equal(t, tt.wantVotingExclusions, esClient.AddVotingConfigExclusionsCalledWith)
			}
			if tt.wantErrOnce != "" {
				_, err := results.Aggregate()
				require.Error(t, err)
				require.Equal(t, 1, strings.Count(err.Error(), tt.wantErrOnce), err.Error())
			}
			require.Equal(t, tt.wantShutdownCancelled, esClient.DeleteShutdownCalled)
			if tt.wantShutdowns != nil {
				require.ElementsMatch(t, tt.wantShutdowns, slices.Collect(maps.Keys(esClient.Shutdowns)))
			}
			// the shutdown status of each node is retrieved, and reported to the observer, once, or twice for a node draining
			// from the start: right after recording the nodes to be removed, and with the shutdowns requested. Both read the
			// retrieved shutdowns, no request is made.
			initiallyDraining := sets.New[string]()
			for _, node := range sets.List(desiredLeavingNodeNames(t.Context(), expectedStatefulSets, actualStatefulSets)) {
				// a node that is not a member of the cluster has no node ID, so its shutdown is not retrieved
				if shutdown, exists := tt.shutdowns["id-"+node]; exists && shutdown.Is(esclient.Remove) && !tt.notJoinedPods.Has(node) {
					initiallyDraining.Insert(node)
				}
			}
			for node, calls := range nodeShutdown.statusCalls {
				wantCalls := 1
				if initiallyDraining.Has(node) {
					wantCalls = 2
				}
				if override, exists := tt.wantStatusCalls[node]; exists {
					wantCalls = override
				}
				require.Equal(t, wantCalls, calls, node)
			}
			if tt.wantReason != "" {
				_, reason := results.IsReconciled()
				require.Equal(t, tt.wantReason, reason)
			}
			for name, wantReplicas := range tt.wantReplicas {
				var statefulSet appsv1.StatefulSet
				require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: name}, &statefulSet))
				require.Equal(t, wantReplicas, sset.GetReplicas(statefulSet))
			}
			shutdownStatuses := map[string]string{}
			for _, node := range reconcileState.DownscaleReporter.Merge(es.Status.DownscaleOperation).Nodes {
				shutdownStatuses[node.Name] = node.ShutdownStatus
			}
			for name, wantStatus := range tt.wantShutdownStatuses {
				require.Equal(t, wantStatus, shutdownStatuses[name], name)
			}
			stalledReported := reconcileState.DownscaleReporter.Merge(es.Status.DownscaleOperation).Stalled
			require.Equal(t, tt.wantStalled, stalledReported != nil && *stalledReported)
			stalledEvents := 0
			var stalledMessage string
			var removedEvents []events.Event
			for _, event := range reconcileState.Events() {
				switch event.Reason {
				case events.EventReasonStalled:
					stalledEvents++
					stalledMessage = event.Message
				case events.EventReasonRemoved:
					removedEvents = append(removedEvents, event)
				}
			}
			require.Len(t, removedEvents, len(tt.wantRemovedEvents))
			wantType := corev1.EventTypeNormal
			if tt.wantRemovedWarning {
				wantType = corev1.EventTypeWarning
			}
			for i, want := range tt.wantRemovedEvents {
				require.Contains(t, removedEvents[i].Message, want)
				require.Equal(t, wantType, removedEvents[i].EventType)
			}
			_, updatedES := reconcileState.Apply()
			if tt.wantStalled {
				require.Equal(t, 1, stalledEvents)
				if tt.wantStalledMessage != "" {
					require.Contains(t, stalledMessage, tt.wantStalledMessage)
				}
				if tt.wantNotStalledMessage != "" {
					require.NotContains(t, stalledMessage, tt.wantNotStalledMessage)
				}
				require.NotNil(t, updatedES)
				require.Equal(t, esv1.ElasticsearchNodeShutdownStalledPhase, updatedES.Status.Phase)
			} else {
				require.Zero(t, stalledEvents)
			}
		})
	}
}

// TestHandleDownscale_unreachedNodesKeepTheirReportedShutdown checks that a leaving node that is missing from a partial
// nodes response keeps the shutdown status it was reported with over several reconciliations, as the upgrades rely on the
// status of the previous reconciliation to leave it alone while it drains. The node is a live draining member that did
// not respond, so it has no node ID: no shutdown is retrieved or requested for it.
func TestHandleDownscale_unreachedNodesKeepTheirReportedShutdown(t *testing.T) {
	tests := []struct {
		name string
		// partial is true if the node that did not respond is reported as failed in the nodes response
		partial bool
		// reported is the status of the node written by the previous reconciliation
		reported esv1.DownscaledNode
		// wantNode is the status reported for the node after each reconciliation
		wantNode    esv1.DownscaledNode
		wantStalled bool
	}{
		{
			name:     "a node missing from a partial nodes response keeps its reported shutdown",
			partial:  true,
			reported: esv1.DownscaledNode{Name: "data-2", ShutdownStatus: "IN_PROGRESS"},
			wantNode: esv1.DownscaledNode{Name: "data-2", ShutdownStatus: "IN_PROGRESS"},
		},
		{
			name:        "a node missing from a partial nodes response keeps its reported stalled shutdown and explanation",
			partial:     true,
			reported:    esv1.DownscaledNode{Name: "data-2", ShutdownStatus: "STALLED", Explanation: new("cannot move")},
			wantNode:    esv1.DownscaledNode{Name: "data-2", ShutdownStatus: "STALLED", Explanation: new("cannot move")},
			wantStalled: true,
		},
		{
			// the node is not a member of the cluster: it has no shutdown to report
			name:     "a node missing from a complete nodes response is reported as not started",
			partial:  false,
			reported: esv1.DownscaledNode{Name: "data-2", ShutdownStatus: "IN_PROGRESS"},
			wantNode: esv1.DownscaledNode{Name: "data-2", ShutdownStatus: "NOT_STARTED"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{Namespace: "ns", Name: clusterName}
			es.Spec.NodeSets = []esv1.NodeSet{{Name: "master", Count: 1}, {Name: "data", Count: 2}}
			// the status written by the reconciliation that requested the shutdown of data-2
			es.Status.DownscaleOperation = esv1.DownscaleOperation{
				LastUpdatedTime: metav1.Now(),
				Nodes:           []esv1.DownscaledNode{tt.reported},
			}
			masterSset := sset.TestSset{Namespace: es.Namespace, Name: "master", ClusterName: es.Name, Master: true, Replicas: 1}
			dataSset := sset.TestSset{Namespace: es.Namespace, Name: "data", ClusterName: es.Name, Data: true, Replicas: 3}
			actualStatefulSets := es_sset.StatefulSetList{masterSset.Build(), dataSset.Build()}
			dataSset.Replicas = 2
			expectedStatefulSets := es_sset.StatefulSetList{masterSset.Build(), dataSset.Build()}

			objs := []client.Object{&es, actualStatefulSets[0].DeepCopy(), actualStatefulSets[1].DeepCopy()}
			podToNodeID := map[string]string{}
			nodes := esclient.Nodes{Nodes: map[string]esclient.Node{}}
			for _, p := range []sset.TestPod{
				{Name: "master-0", StatefulSetName: "master", Master: true},
				{Name: "data-0", StatefulSetName: "data", Data: true},
				{Name: "data-1", StatefulSetName: "data", Data: true},
				// data-2 is draining: its readiness port is closed
				{Name: "data-2", StatefulSetName: "data", Data: true, Ready: false},
			} {
				p.Namespace, p.ClusterName, p.Ready = es.Namespace, es.Name, p.Ready || p.Name != "data-2"
				objs = append(objs, p.BuildPtr())
				if p.Name == "data-2" {
					continue
				}
				podToNodeID[p.Name] = "id-" + p.Name
				nodes.Nodes["id-"+p.Name] = esclient.Node{Name: p.Name}
			}
			if tt.partial {
				nodes.Summary.Failed = 1
			}
			k8sClient := k8s.NewFakeClient(objs...)
			esClient := &fakeESClient{
				version: version.MustParse("8.0.0"), nodes: nodes,
				Shutdowns: map[string]esclient.NodeShutdown{
					"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
				},
			}

			// each reconciliation relies on the status written by the previous one
			for reconciliation := range 3 {
				reconcileState := reconcile.MustNewState(es)
				downscaleCtx := downscaleContext{
					k8sClient:      k8sClient,
					expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
					reconcileState: reconcileState,
					nodeShutdown: shutdown.WithObserver(
						shutdown.NewNodeShutdown(esClient, podToNodeID, esclient.Remove, "", nil, logr.Discard()),
						reconcileState.DownscaleReporter,
					),
					esClient:  esClient,
					es:        es,
					parentCtx: context.Background(),
				}
				leaving := leavingNodesOfStatus(es, expectedStatefulSets, actualStatefulSets, esClient, downscaleCtx.nodeShutdown)
				results := HandleDownscale(downscaleCtx, expectedStatefulSets, actualStatefulSets, leaving)
				require.False(t, results.HasError(), "reconciliation %d", reconciliation)

				es.Status.DownscaleOperation = reconcileState.DownscaleReporter.Merge(es.Status.DownscaleOperation)
				require.Equal(t, []esv1.DownscaledNode{tt.wantNode}, es.Status.DownscaleOperation.Nodes, "reconciliation %d", reconciliation)
				require.Equal(t, tt.wantStalled, es.Status.DownscaleOperation.Stalled != nil && *es.Status.DownscaleOperation.Stalled, "reconciliation %d", reconciliation)
				// a stalled status comes with the stalled phase and event
				stalledEvents := 0
				for _, event := range reconcileState.Events() {
					if event.Reason == events.EventReasonStalled {
						stalledEvents++
					}
				}
				if tt.wantStalled {
					require.Equal(t, 1, stalledEvents, "reconciliation %d", reconciliation)
					_, updatedES := reconcileState.Apply()
					require.NotNil(t, updatedES, "reconciliation %d", reconciliation)
					require.Equal(t, esv1.ElasticsearchNodeShutdownStalledPhase, updatedES.Status.Phase, "reconciliation %d", reconciliation)
				} else {
					require.Zero(t, stalledEvents, "reconciliation %d", reconciliation)
				}

				// the node is held and its shutdown is not cancelled
				require.False(t, esClient.DeleteShutdownCalled, "reconciliation %d", reconciliation)
				var statefulSet appsv1.StatefulSet
				require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: "data"}, &statefulSet))
				require.Equal(t, int32(3), sset.GetReplicas(statefulSet), "reconciliation %d", reconciliation)

				// the upgrades leave the node alone only while its shutdown is reported
				unreached, err := leavingNodesOfStatus(es, expectedStatefulSets, actualStatefulSets, esClient, downscaleCtx.nodeShutdown).unreached([]string{"data-2"})
				require.NoError(t, err)
				require.Equal(t, tt.partial, unreached.Has("data-2"), "reconciliation %d", reconciliation)
			}
		})
	}
}

// leavingNodesOfStatus returns the leavingNodes of the next reconciliation, which relies on the downscale status of the
// given Elasticsearch.
func leavingNodesOfStatus(
	es esv1.Elasticsearch,
	expected, actual es_sset.StatefulSetList,
	esClient esclient.Client,
	nodeShutdown shutdown.Interface,
) *leavingNodes {
	return newLeavingNodes(
		desiredLeavingNodeNames(context.Background(), expected, actual),
		NewMemoizingESState(context.Background(), esClient), nodeShutdown, es.Status.DownscaleOperation,
	)
}

func Test_calculateDownscales(t *testing.T) {
	ssets := es_sset.StatefulSetList{
		{
			Namespace: "ns",
			Name:      "sset0",
			Spec: appsv1.StatefulSetSpec{
				Replicas: new(int32(3)),
			},
		},
		{
			Namespace: "ns",
			Name:      "sset1",
			Spec: appsv1.StatefulSetSpec{
				Replicas: new(int32(3))},
		},
		{
			Namespace: "ns",
			Name:      "sset2",
			Spec: appsv1.StatefulSetSpec{
				Replicas: new(int32(3))},
		},
	}

	var tests = []struct {
		name                 string
		expectedStatefulSets es_sset.StatefulSetList
		actualStatefulSets   es_sset.StatefulSetList
		wantDownscales       []ssetDownscale
		wantDeletions        es_sset.StatefulSetList
	}{
		{
			name:               "no actual statefulset: nothing to do",
			actualStatefulSets: nil,
			wantDownscales:     nil,
			wantDeletions:      nil,
		},

		{
			name: "upscale: nothing to do",
			expectedStatefulSets: es_sset.StatefulSetList{
				{
					Namespace: "ns",
					Name:      "sset0",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(4)),
					},
				},
				{
					Namespace: "ns",
					Name:      "sset1",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(5))},
				},
				{
					Namespace: "ns",
					Name:      "sset2",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(3))},
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
			expectedStatefulSets: es_sset.StatefulSetList{
				{
					Namespace: "ns",
					Name:      "sset0",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(3)),
					},
				},
				{
					Namespace: "ns",
					Name:      "sset1",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(2))},
				},
				{
					Namespace: "ns",
					Name:      "sset2",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(1))},
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
			expectedStatefulSets: es_sset.StatefulSetList{
				{
					Namespace: "ns",
					Name:      "sset2",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(1))},
				},
				{
					Namespace: "ns",
					Name:      "sset3",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
			},
			actualStatefulSets: es_sset.StatefulSetList{
				// statefulset with 0 replicas which has no corresponding expected statefulset: should be deleted
				{
					Namespace: "ns",
					Name:      "sset1",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
				// statefulset with 0 replicas which has a corresponding expected statefulset with 1 replica: should be kept
				{
					Namespace: "ns",
					Name:      "sset2",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
				// statefulset with 1 replicas that should be downscaled to 0
				{
					Namespace: "ns",
					Name:      "sset3",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(1))},
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
							Replicas: new(int32(1))},
					},
					initialReplicas: 1,
					targetReplicas:  0,
					finalReplicas:   0,
				},
			},
			wantDeletions: es_sset.StatefulSetList{
				{
					Namespace: "ns",
					Name:      "sset1",
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
			},
		},
		{
			name: "do not delete actual statefulsets with 0 replicas if referenced by a nodeSet",
			expectedStatefulSets: es_sset.StatefulSetList{
				{
					Namespace: "ns",
					Name:      esv1.StatefulSet(clusterName, "nodeset-2"),
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(1))},
				},
				{
					Namespace: "ns",
					Name:      esv1.StatefulSet(clusterName, "nodeset-3"),
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
			},
			actualStatefulSets: es_sset.StatefulSetList{
				// statefulset with 0 replicas which has no corresponding expected statefulset and is not referenced through a nodeSet: should be deleted
				{
					Namespace: "ns",
					Name:      esv1.StatefulSet(clusterName, "nodeset-1"),
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
				// statefulset with 0 replicas which has a corresponding expected statefulset with 0 replica but is used by a nodeSet: should be kept
				{
					Namespace: "ns",
					Name:      esv1.StatefulSet(clusterName, "nodeset-3"),
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
			},
			wantDownscales: nil, // No downscale expected
			wantDeletions: es_sset.StatefulSetList{
				{
					Namespace: "ns",
					Name:      esv1.StatefulSet(clusterName, "nodeset-1"),
					Spec: appsv1.StatefulSetSpec{
						Replicas: new(int32(0))},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDownscales, gotDeletions, gotHeldReasons := calculateDownscales(context.Background(), downscaleState{}, expectedReplicasOfStatefulSets(tt.expectedStatefulSets), tt.actualStatefulSets, downscaleBudgetFilter)
			require.Equal(t, tt.wantDownscales, gotDownscales)
			require.Equal(t, tt.wantDeletions, gotDeletions)
			// no budget limit and no master removal in progress: nothing is held
			require.Empty(t, gotHeldReasons)
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
				ctx: downscaleContext{
					parentCtx: context.Background(),
				},
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
			// no shutdown interface: reading a status for the node would panic
			name: "downscale possible without a shutdown for a removable node that is not a member of the cluster",
			args: args{
				ctx: downscaleContext{parentCtx: context.Background(), reconcileState: reconcile.MustNewState(esv1.Elasticsearch{})},
				downscale: ssetDownscale{
					statefulSet:         appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
					initialReplicas:     3,
					targetReplicas:      2,
					finalReplicas:       2,
					removableNonMembers: map[string]nonMember{"default-2": {cannotJoin: true}},
				},
			},
			want: ssetDownscale{
				statefulSet:         appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				initialReplicas:     3,
				targetReplicas:      2,
				finalReplicas:       2,
				removableNonMembers: map[string]nonMember{"default-2": {cannotJoin: true}},
			},
		},
		{
			name: "downscale possible from 3 to 2",
			args: args{
				ctx: downscaleContext{
					parentCtx:    context.Background(),
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
					parentCtx:      context.Background(),
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
		{
			name: "downscale not possible: pending shard activity",
			args: args{
				ctx: downscaleContext{
					parentCtx:      context.Background(),
					reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
					nodeShutdown:   migration.NewShardMigration(es, &fakeESClient{}, migration.NewFakeShardListerWithShardActivity(esclient.Shards{})),
				},
				downscale: ssetDownscale{
					initialReplicas: 3,
					targetReplicas:  2,
					finalReplicas:   2,
				},
			},
			want: ssetDownscale{
				initialReplicas: 3,
				targetReplicas:  3,
				finalReplicas:   2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// shard migration registers no shutdown: the statuses are retrieved by calculatePerformableDownscale
			got, err := calculatePerformableDownscale(tt.args.ctx, tt.args.downscale, nil)
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
		statefulSets            es_sset.StatefulSetList
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
			state: &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1))},
			statefulSets: es_sset.StatefulSetList{
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
			state: &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0))},
			statefulSets: es_sset.StatefulSetList{
				sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
			},
			expectedStatefulSets: []appsv1.StatefulSet{
				sset.TestSset{Name: "default", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(),
			},
			expectedDownscaledNodes: nil, // expectedDownscaledNodes is not updated
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runtimeObjs []client.Object
			for i := range tt.statefulSets {
				runtimeObjs = append(runtimeObjs, &tt.statefulSets[i])
			}
			k8sClient := k8s.NewFakeClient(runtimeObjs...)
			esState := reconcile.MustNewState(esv1.Elasticsearch{})
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
				nodeShutdown:   shutdown.WithObserver(migration.NewShardMigration(es, &fakeESClient{}, migration.NewFakeShardLister(esclient.Shards{})), esState),
				esClient:       &fakeESClient{},
				parentCtx:      context.Background(),
			}
			// do the downscale
			// shard migration registers no shutdown: the statuses are retrieved by calculatePerformableDownscale
			_, err := attemptDownscale(downscaleCtx, tt.downscale, nil)
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
		expectations: expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
		esClient:     &fakeESClient{},
		parentCtx:    context.Background(),
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
	err := doDownscale(downscaleCtx, downscale)
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
				Namespace: es.Namespace,
				Labels: map[string]string{
					label.ClusterNameLabelName:             es.Name,
					string(label.NodeTypesMasterLabelName): "true",
					label.VersionLabelName:                 "7.1.0",
					label.StatefulSetNameLabelName:         ssetMasters.Name,
				},
			}
			k8sClient := k8s.NewFakeClient(es.DeepCopy(), &ssetMasters, &ssetData, &v7Pod)
			esClient := &fakeESClient{}
			downscaleCtx := downscaleContext{
				k8sClient:      k8sClient,
				expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				reconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
				esClient:       esClient,
				es:             es,
				parentCtx:      context.Background(),
			}
			// do the downscale
			err := doDownscale(downscaleCtx, tt.downscale)
			require.NoError(t, err)
			// check call to zen2 is the expected one
			require.Equal(t, tt.wantZen2Called, esClient.AddVotingConfigExclusionsCalled)
			require.Equal(t, tt.wantZen2CalledWith, esClient.AddVotingConfigExclusionsCalledWith)
		})
	}
}

func Test_deleteStatefulSetResources(t *testing.T) {
	es := esv1.Elasticsearch{Namespace: "ns", Name: "cluster"}
	sset := sset.TestSset{Namespace: "ns", Name: "sset", ClusterName: es.Name}.Build()
	cfg := settings.ConfigSecret(es, sset.Name, []byte("fake config data"), metadata.Metadata{})
	svc := nodespec.HeadlessService(&es, sset.Name, metadata.Metadata{})

	tests := []struct {
		name      string
		resources []client.Object
	}{
		{
			name:      "happy path: delete 3 resources",
			resources: []client.Object{&es, &sset, &cfg, &svc},
		},
		{
			name:      "cfg and service were already deleted: should not return an error",
			resources: []client.Object{&es, &sset},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(tt.resources...)
			err := deleteStatefulSetResources(context.Background(), k8sClient, es, sset)
			require.NoError(t, err)
			// sset, cfg and headless services should not be there anymore
			require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&sset), &sset)))
			require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&cfg), &cfg)))
			require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&svc), &svc)))
		})
	}
}

func Test_deleteStatefulSets(t *testing.T) {
	es := esv1.Elasticsearch{Namespace: "ns", Name: "cluster"}
	tests := []struct {
		name          string
		toDelete      es_sset.StatefulSetList
		objs          []client.Object
		wantRemaining es_sset.StatefulSetList
		wantErr       func(err error) bool
	}{
		{
			name:     "nothing to delete",
			toDelete: nil,
			objs: []client.Object{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
			},
			wantRemaining: es_sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name, ResourceVersion: "999"}.Build(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "two StatefulSets to delete",
			toDelete: es_sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.Build(),
				sset.TestSset{Namespace: "ns", Name: "sset3", ClusterName: es.Name}.Build(),
			},
			objs: []client.Object{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
				sset.TestSset{Namespace: "ns", Name: "sset3", ClusterName: es.Name, ResourceVersion: "999"}.BuildPtr(),
			},
			wantRemaining: es_sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset2", ClusterName: es.Name, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "statefulSet already deleted",
			toDelete: es_sset.StatefulSetList{
				sset.TestSset{Namespace: "ns", Name: "sset1", ClusterName: es.Name}.Build(),
			},
			objs:          []client.Object{},
			wantRemaining: es_sset.StatefulSetList{},
			wantErr:       apierrors.IsNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.objs...)
			err := deleteStatefulSets(context.Background(), tt.toDelete, client, es)
			if tt.wantErr != nil {
				require.True(t, tt.wantErr(err))
			} else {
				require.NoError(t, err)
			}
			var remainingSsets appsv1.StatefulSetList
			require.NoError(t, client.List(context.Background(), &remainingSsets))
			require.Equal(t, tt.wantRemaining, es_sset.StatefulSetList(remainingSsets.Items))
		})
	}
}

func Test_clusterIsSettled(t *testing.T) {
	nodes := esclient.Nodes{Nodes: map[string]esclient.Node{"id-a": {Name: "a"}, "id-b": {Name: "b"}}}
	green := esclient.Health{Status: esv1.ElasticsearchGreenHealth, NumberOfNodes: 2}
	tests := []struct {
		name string
		// memoizedHealth, if set, is the health that the state memoized before the current one
		memoizedHealth *esclient.Health
		health         esclient.Health
		// currentNodes, if set, are the nodes once the health is retrieved, which are the memoized ones otherwise
		currentNodes *esclient.Nodes
		nodesErr     error
		want         bool
	}{
		{
			name:   "green with no shard activity and the same nodes",
			health: green,
			want:   true,
		},
		{
			name:           "the memoized health is green, but the current one is not",
			memoizedHealth: &green,
			health:         esclient.Health{Status: esv1.ElasticsearchYellowHealth, NumberOfNodes: 2},
		},
		{
			name:   "a node joined after the nodes were retrieved",
			health: esclient.Health{Status: esv1.ElasticsearchGreenHealth, NumberOfNodes: 3},
		},
		{
			// the health counts c, which may hold shards of a node removed without a shutdown
			name:         "a node joined while another one left after the nodes were retrieved",
			health:       green,
			currentNodes: &esclient.Nodes{Nodes: map[string]esclient.Node{"id-a": {Name: "a"}, "id-c": {Name: "c"}}},
		},
		{
			name:         "the nodes response is partial once the health is retrieved",
			health:       green,
			currentNodes: &esclient.Nodes{Summary: esclient.NodesSummary{Failed: 1}, Nodes: nodes.Nodes},
		},
		{
			name:     "the nodes cannot be retrieved once the health is retrieved",
			health:   green,
			nodesErr: errors.New("nodes failed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esClient := &fakeESClient{nodes: nodes, health: tt.health}
			esState := NewMemoizingESState(context.Background(), esClient)
			if tt.memoizedHealth != nil {
				esClient.health = *tt.memoizedHealth
				_, err := esState.Health()
				require.NoError(t, err)
				esClient.health = tt.health
			}
			// memoize the nodes, then change them for the requests that follow
			_, err := esState.NodeNameToID()
			require.NoError(t, err)
			if tt.currentNodes != nil {
				esClient.nodes = *tt.currentNodes
			}
			esClient.GetNodesErr = tt.nodesErr
			assert.Equal(t, tt.want, clusterIsSettled(context.Background(), esClient, esState))
		})
	}
}
