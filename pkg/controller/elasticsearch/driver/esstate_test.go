// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
)

// -- ES Client mock

// fakeESClient mocks the ES client to register function calls that were made.
// It's also used in other test files from this package.
type fakeESClient struct { //nolint:maligned
	esclient.Client

	SetMinimumMasterNodesCalled     bool
	SetMinimumMasterNodesCalledWith int

	AddVotingConfigExclusionsCalled     bool
	AddVotingConfigExclusionsCalledWith []string

	ExcludeFromShardAllocationCalled     bool
	ExcludeFromShardAllocationCalledWith string

	DisableReplicaShardsAllocationCalled bool

	EnableShardAllocationCalled bool

	SyncedFlushCalled bool
	FlushCalled       bool

	nodes             esclient.Nodes
	GetNodesCallCount int

	clusterRoutingAllocation             esclient.ClusterRoutingAllocation
	GetClusterRoutingAllocationCallCount int

	Shutdowns            map[string]esclient.NodeShutdown
	DeleteShutdownCalled bool

	health                      esclient.Health
	GetClusterHealthCalledCount int
	version                     version.Version
}

func (f *fakeESClient) SetMinimumMasterNodes(_ context.Context, n int) error {
	f.SetMinimumMasterNodesCalled = true
	f.SetMinimumMasterNodesCalledWith = n
	return nil
}

func (f *fakeESClient) AddVotingConfigExclusions(_ context.Context, nodeNames []string) error {
	f.AddVotingConfigExclusionsCalled = true
	f.AddVotingConfigExclusionsCalledWith = append(f.AddVotingConfigExclusionsCalledWith, nodeNames...)
	return nil
}

func (f *fakeESClient) ExcludeFromShardAllocation(_ context.Context, nodes string) error {
	f.ExcludeFromShardAllocationCalled = true
	f.ExcludeFromShardAllocationCalledWith = nodes
	return nil
}

func (f *fakeESClient) DisableReplicaShardsAllocation(_ context.Context) error {
	f.DisableReplicaShardsAllocationCalled = true
	return nil
}

func (f *fakeESClient) EnableShardAllocation(_ context.Context) error {
	f.EnableShardAllocationCalled = true
	return nil
}

func (f *fakeESClient) SyncedFlush(_ context.Context) error {
	f.SyncedFlushCalled = true
	return nil
}

func (f *fakeESClient) Flush(_ context.Context) error {
	f.FlushCalled = true
	return nil
}

func (f *fakeESClient) GetNodes(_ context.Context) (esclient.Nodes, error) {
	f.GetNodesCallCount++
	return f.nodes, nil
}

func (f *fakeESClient) GetClusterRoutingAllocation(_ context.Context) (esclient.ClusterRoutingAllocation, error) {
	f.GetClusterRoutingAllocationCallCount++
	return f.clusterRoutingAllocation, nil
}

func (f *fakeESClient) GetClusterHealth(_ context.Context) (esclient.Health, error) {
	f.GetClusterHealthCalledCount++
	return f.health, nil
}

func (f *fakeESClient) GetClusterHealthWaitForAllEvents(_ context.Context) (esclient.Health, error) {
	f.GetClusterHealthCalledCount++
	return f.health, nil
}

func (f *fakeESClient) PutShutdown(_ context.Context, _ string, _ esclient.ShutdownType, _ string) error {
	return nil
}

func (f *fakeESClient) GetShutdown(_ context.Context, nodeID *string) (esclient.ShutdownResponse, error) {
	var ns []esclient.NodeShutdown //nolint:prealloc
	for k, v := range f.Shutdowns {
		if nodeID != nil && k != *nodeID {
			continue
		}
		ns = append(ns, v)
	}
	return esclient.ShutdownResponse{Nodes: ns}, nil
}

func (f *fakeESClient) DeleteShutdown(_ context.Context, _ string) error {
	f.DeleteShutdownCalled = true
	return nil
}

func (f *fakeESClient) Version() version.Version {
	return f.version
}

// -- ESState tests

func Test_memoizingNodes_NodesInCluster(t *testing.T) {
	esClient := &fakeESClient{
		nodes: esclient.Nodes{Nodes: map[string]esclient.Node{"a": {Name: "a"}, "b": {Name: "b"}, "c": {Name: "c"}}},
	}
	memoizingNodes := &memoizingNodes{esClient: esClient, ctx: context.Background()}

	inCluster, err := memoizingNodes.NodesInCluster([]string{"a", "b", "c"})
	require.NoError(t, err)
	// es should be requested on first call
	require.Equal(t, 1, esClient.GetNodesCallCount)
	// nodes are in the cluster
	require.Equal(t, true, inCluster)
	// ES should not be requested again on subsequent calls
	inCluster, err = memoizingNodes.NodesInCluster([]string{"a", "b", "c"})
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetNodesCallCount)
	require.Equal(t, true, inCluster)

	// nodes are a subset of the cluster nodes: should return true
	inCluster, err = memoizingNodes.NodesInCluster([]string{"a", "b"})
	require.NoError(t, err)
	require.True(t, inCluster)

	// all nodes are not in the cluster: should return false
	inCluster, err = memoizingNodes.NodesInCluster([]string{"a", "b", "c", "e"})
	require.NoError(t, err)
	require.False(t, inCluster)
}

func Test_memoizingShardsAllocationEnabled_ShardAllocationsEnabled(t *testing.T) {
	// with cluster routing allocation enabled (by default)
	esClient := &fakeESClient{}
	s := &memoizingShardsAllocationEnabled{esClient: esClient, ctx: context.Background()}

	enabled, err := s.ShardAllocationsEnabled()
	require.NoError(t, err)
	// es should be requested on first call
	require.Equal(t, 1, esClient.GetClusterRoutingAllocationCallCount)
	require.True(t, enabled)
	// ES should not be requested again on subsequent calls
	enabled, err = s.ShardAllocationsEnabled()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterRoutingAllocationCallCount)
	require.True(t, enabled)

	// simulate cluster routing allocation disabled
	clusterRoutingAllocation := esclient.ClusterRoutingAllocation{}
	clusterRoutingAllocation.Transient.Cluster.Routing.Allocation.Enable = "none"
	esClient = &fakeESClient{clusterRoutingAllocation: clusterRoutingAllocation}
	s = &memoizingShardsAllocationEnabled{esClient: esClient, ctx: context.Background()}
	enabled, err = s.ShardAllocationsEnabled()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterRoutingAllocationCallCount)
	require.False(t, enabled)
}

func Test_memoizingGreenHealth_GreenHealth(t *testing.T) {
	esClient := &fakeESClient{health: esclient.Health{Status: esv1.ElasticsearchGreenHealth}}
	h := &memoizingHealth{esClient: esClient, ctx: context.Background()}

	health, err := h.Health()
	require.NoError(t, err)
	// es should be requested on first call
	require.Equal(t, 1, esClient.GetClusterHealthCalledCount)
	require.Equal(t, esv1.ElasticsearchGreenHealth, health.Status)
	// ES should not be requested again on subsequent calls
	health, err = h.Health()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterHealthCalledCount)
	require.Equal(t, esv1.ElasticsearchGreenHealth, health.Status)

	// simulate yellow health
	esClient = &fakeESClient{health: esclient.Health{Status: esv1.ElasticsearchYellowHealth}}
	h = &memoizingHealth{esClient: esClient, ctx: context.Background()}
	health, err = h.Health()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterHealthCalledCount)
	require.NotEqual(t, esv1.ElasticsearchGreenHealth, health.Status)
}

func TestNewMemoizingESState(t *testing.T) {
	esClient := &fakeESClient{}
	// just make sure everything is initialized correctly (no panic for nil pointers)
	s := NewMemoizingESState(context.Background(), esClient)
	_, err := s.Health()
	require.NoError(t, err)
	_, err = s.ShardAllocationsEnabled()
	require.NoError(t, err)
	_, err = s.NodesInCluster([]string{"a"})
	require.NoError(t, err)
}
