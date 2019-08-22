// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
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

	EnableShardAllocationCalled bool

	SyncedFlushCalled bool

	nodes             esclient.Nodes
	GetNodesCallCount int

	clusterRoutingAllocation             esclient.ClusterRoutingAllocation
	GetClusterRoutingAllocationCallCount int

	health                      esclient.Health
	GetClusterHealthCalledCount int
}

func (f *fakeESClient) SetMinimumMasterNodes(ctx context.Context, n int) error {
	f.SetMinimumMasterNodesCalled = true
	f.SetMinimumMasterNodesCalledWith = n
	return nil
}

func (f *fakeESClient) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	f.AddVotingConfigExclusionsCalled = true
	f.AddVotingConfigExclusionsCalledWith = append(f.AddVotingConfigExclusionsCalledWith, nodeNames...)
	return nil
}

func (f *fakeESClient) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
	f.ExcludeFromShardAllocationCalled = true
	f.ExcludeFromShardAllocationCalledWith = nodes
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

func (f *fakeESClient) GetNodes(ctx context.Context) (esclient.Nodes, error) {
	f.GetNodesCallCount++
	return f.nodes, nil
}

func (f *fakeESClient) GetClusterRoutingAllocation(ctx context.Context) (esclient.ClusterRoutingAllocation, error) {
	f.GetClusterRoutingAllocationCallCount++
	return f.clusterRoutingAllocation, nil
}

func (f *fakeESClient) GetClusterHealth(ctx context.Context) (esclient.Health, error) {
	f.GetClusterHealthCalledCount++
	return f.health, nil
}

// -- ESState tests

func Test_lazyNodes_NodesInCluster(t *testing.T) {
	esClient := &fakeESClient{
		nodes: esclient.Nodes{Nodes: map[string]esclient.Node{"a": {Name: "a"}, "b": {Name: "b"}, "c": {Name: "c"}}},
	}
	lazyNodes := &lazyNodes{esClient: esClient}

	inCluster, err := lazyNodes.NodesInCluster([]string{"a", "b", "c"})
	require.NoError(t, err)
	// es should be requested on first call
	require.Equal(t, 1, esClient.GetNodesCallCount)
	// nodes are in the cluster
	require.Equal(t, true, inCluster)
	// ES should not be requested again on subsequent calls
	inCluster, err = lazyNodes.NodesInCluster([]string{"a", "b", "c"})
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetNodesCallCount)
	require.Equal(t, true, inCluster)

	// nodes are a subset of the cluster nodes: should return true
	inCluster, err = lazyNodes.NodesInCluster([]string{"a", "b"})
	require.NoError(t, err)
	require.True(t, inCluster)

	// all nodes are not in the cluster: should return false
	inCluster, err = lazyNodes.NodesInCluster([]string{"a", "b", "c", "e"})
	require.NoError(t, err)
	require.False(t, inCluster)
}

func Test_lazyShardsAllocationEnabled_ShardAllocationsEnabled(t *testing.T) {
	// with cluster routing allocation enabled (by default)
	esClient := &fakeESClient{}
	l := &lazyShardsAllocationEnabled{esClient: esClient}

	enabled, err := l.ShardAllocationsEnabled()
	require.NoError(t, err)
	// es should be requested on first call
	require.Equal(t, 1, esClient.GetClusterRoutingAllocationCallCount)
	require.True(t, enabled)
	// ES should not be requested again on subsequent calls
	enabled, err = l.ShardAllocationsEnabled()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterRoutingAllocationCallCount)
	require.True(t, enabled)

	// simulate cluster routing allocation disabled
	clusterRoutingAllocation := esclient.ClusterRoutingAllocation{}
	clusterRoutingAllocation.Transient.Cluster.Routing.Allocation.Enable = "none"
	esClient = &fakeESClient{clusterRoutingAllocation: clusterRoutingAllocation}
	l = &lazyShardsAllocationEnabled{esClient: esClient}
	enabled, err = l.ShardAllocationsEnabled()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterRoutingAllocationCallCount)
	require.False(t, enabled)
}

func Test_lazyGreenHealth_GreenHealth(t *testing.T) {
	esClient := &fakeESClient{health: esclient.Health{Status: string(v1alpha1.ElasticsearchGreenHealth)}}
	l := &lazyGreenHealth{esClient: esClient}

	green, err := l.GreenHealth()
	require.NoError(t, err)
	// es should be requested on first call
	require.Equal(t, 1, esClient.GetClusterHealthCalledCount)
	require.True(t, green)
	// ES should not be requested again on subsequent calls
	green, err = l.GreenHealth()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterHealthCalledCount)
	require.True(t, green)

	// simulate yellow health
	esClient = &fakeESClient{health: esclient.Health{Status: string(v1alpha1.ElasticsearchYellowHealth)}}
	l = &lazyGreenHealth{esClient: esClient}
	green, err = l.GreenHealth()
	require.NoError(t, err)
	require.Equal(t, 1, esClient.GetClusterHealthCalledCount)
	require.False(t, green)
}

func TestNewLazyESState(t *testing.T) {
	esClient := &fakeESClient{}
	// just make sure everything is initialized correctly (no panic for nil pointers)
	s := NewLazyESState(esClient)
	_, err := s.GreenHealth()
	require.NoError(t, err)
	_, err = s.ShardAllocationsEnabled()
	require.NoError(t, err)
	_, err = s.NodesInCluster([]string{"a"})
	require.NoError(t, err)
}
