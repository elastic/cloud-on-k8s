// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func Test_cache_Forget(t *testing.T) {
	type fields struct {
		mu     sync.RWMutex
		states map[types.NamespacedName]*cachedState
	}
	type args struct {
		es types.NamespacedName
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cache{
				mu:     tt.fields.mu,
				states: tt.fields.states,
			}
			c.Forget(tt.args.es)
		})
	}
}

type fakeESClient struct {
	Client
	err error

	excludeFromShardAllocationCalled, setMinimumMasterNodesCalled,
	addVotingConfigExclusionsCalled, deleteVotingConfigExclusionsCalled int

	excludedNodesFromShardAllocation string
	minimumMasterNodes               int
	votingConfigExclusions           []string
}

func (f *fakeESClient) ExcludeFromShardAllocation(_ context.Context, nodes string) error {
	f.excludeFromShardAllocationCalled++
	f.excludedNodesFromShardAllocation = nodes
	return f.err
}

func (f *fakeESClient) SetMinimumMasterNodes(_ context.Context, n int) error {
	f.setMinimumMasterNodesCalled++
	f.minimumMasterNodes = n
	return f.err
}

func (f *fakeESClient) AddVotingConfigExclusions(_ context.Context, nodeNames []string, _ string) error {
	f.addVotingConfigExclusionsCalled++
	f.votingConfigExclusions = nodeNames
	return f.err
}

func (f *fakeESClient) DeleteVotingConfigExclusions(_ context.Context, _ bool) error {
	f.deleteVotingConfigExclusionsCalled++
	f.votingConfigExclusions = nil
	return f.err
}

var (
	es1 = types.NamespacedName{
		Name:      "es1",
		Namespace: "ns",
	}
	es2 = types.NamespacedName{
		Name:      "es2",
		Namespace: "ns",
	}
)

func stringPtr(value string) *string {
	return &value
}

func Test_cachedClient_DeleteVotingConfigExclusions(t *testing.T) {
	cacheClientBuilder := NewCachedClientBuilder()
	es1FakeClient := fakeESClient{}

	// DeleteVotingConfigExclusions should be called on a new client
	es1FakeCachedClient := cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.DeleteVotingConfigExclusions(nil, false))
	assert.Equal(t, 1, es1FakeClient.deleteVotingConfigExclusionsCalled)

	// second call should hit the cache
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.DeleteVotingConfigExclusions(nil, false))
	assert.Equal(t, 1, es1FakeClient.deleteVotingConfigExclusionsCalled)

	// Set a config exclusion
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.AddVotingConfigExclusions(nil, []string{"foo"}, ""))
	assert.Equal(t, 1, es1FakeClient.addVotingConfigExclusionsCalled)
	assert.Equal(t, []string{"foo"}, es1FakeClient.votingConfigExclusions)

	// call returns an error
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	es1FakeClient.err = fmt.Errorf("error")
	assert.Error(t, es1FakeCachedClient.DeleteVotingConfigExclusions(nil, false))
	assert.Equal(t, 2, es1FakeClient.deleteVotingConfigExclusionsCalled)

	es1FakeClient.err = nil
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.DeleteVotingConfigExclusions(nil, false))
	assert.Equal(t, 3, es1FakeClient.deleteVotingConfigExclusionsCalled)
	assert.Equal(t, []string(nil), es1FakeClient.votingConfigExclusions)

}

func Test_cachedClient_AddVotingConfigExclusions(t *testing.T) {
	cacheClientBuilder := NewCachedClientBuilder()
	es1FakeClient := fakeESClient{}

	// AddVotingConfigExclusions should be called on a new client
	es1FakeCachedClient := cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.AddVotingConfigExclusions(nil, []string{"foo"}, ""))
	assert.Equal(t, 1, es1FakeClient.addVotingConfigExclusionsCalled)
	assert.Equal(t, []string{"foo"}, es1FakeClient.votingConfigExclusions)

	// second call should hit the cache
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.AddVotingConfigExclusions(nil, []string{"foo"}, ""))
	assert.Equal(t, 1, es1FakeClient.addVotingConfigExclusionsCalled)
	assert.Equal(t, []string{"foo"}, es1FakeClient.votingConfigExclusions)

	// Update config exclusion
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.NoError(t, es1FakeCachedClient.AddVotingConfigExclusions(nil, []string{"foo", "bar"}, ""))
	assert.Equal(t, 2, es1FakeClient.addVotingConfigExclusionsCalled)
	assert.Equal(t, []string{"bar", "foo"}, es1FakeClient.votingConfigExclusions)

	// call returns an error
	es1FakeClient.err = fmt.Errorf("error")
	es1FakeCachedClient = cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)
	assert.Error(t, es1FakeCachedClient.AddVotingConfigExclusions(nil, []string{"foo"}, ""))
	assert.Equal(t, 3, es1FakeClient.addVotingConfigExclusionsCalled)

	es1FakeClient.err = nil
	assert.NoError(t, es1FakeCachedClient.AddVotingConfigExclusions(nil, []string{"foo", "bar"}, ""))
	assert.Equal(t, 4, es1FakeClient.addVotingConfigExclusionsCalled)

}

func Test_cachedClient_ExcludeFromShardAllocation(t *testing.T) {
	cacheClientBuilder := NewCachedClientBuilder()

	es1FakeClient := fakeESClient{}
	es1FakeCachedClient := cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)

	es2FakeClient := fakeESClient{}
	es2FakeCachedClient := cacheClientBuilder.NewElasticsearchCachedClient(es2, &es2FakeClient)

	steps := []struct {
		es1Value, es2Value                   *string
		es1ExpectedValue, es2ExpectedValue   *string
		es1ExpectedCalled, es2ExpectedCalled int
		es1Error, es2Error                   error
	}{
		{
			es1Value:          stringPtr("node1"),
			es1ExpectedValue:  stringPtr("node1"),
			es1ExpectedCalled: 1,
			es2ExpectedCalled: 0,
		},
		{
			es1Value:          stringPtr("node1"),
			es1ExpectedValue:  stringPtr("node1"),
			es2Value:          stringPtr("node2"),
			es2ExpectedValue:  stringPtr("node2"),
			es1ExpectedCalled: 1, // same value, es1 is not called
			es2ExpectedCalled: 1,
		},
		{
			es1Value:          stringPtr("node1_2"),
			es1ExpectedValue:  stringPtr("node1_2"),
			es1ExpectedCalled: 2,
			es2ExpectedCalled: 1,
		},
		{
			es1Value:          stringPtr("node1"),
			es1Error:          fmt.Errorf("error"),
			es2ExpectedValue:  stringPtr("node2"),
			es1ExpectedCalled: 3,
			es2ExpectedCalled: 1,
		},
		{
			es1Value:          stringPtr("node1_2"),
			es1ExpectedValue:  stringPtr("node1_2"),
			es2ExpectedValue:  stringPtr("node2"), // es2 value should still be in memory
			es1ExpectedCalled: 4,                  // API must be called after the error at the previous step
			es2ExpectedCalled: 1,
		},
	}

	for _, step := range steps {
		// Set error field in fake client
		es1FakeClient.err = step.es1Error
		es2FakeClient.err = step.es2Error

		// Simulate API calls
		if step.es1Value != nil {
			_ = es1FakeCachedClient.ExcludeFromShardAllocation(nil, *step.es1Value)
		}
		if step.es2Value != nil {
			_ = es2FakeCachedClient.ExcludeFromShardAllocation(nil, *step.es2Value)
		}
		assert.Equal(t, step.es1ExpectedCalled, es1FakeClient.excludeFromShardAllocationCalled)
		assert.Equal(t, step.es2ExpectedCalled, es2FakeClient.excludeFromShardAllocationCalled)
		if step.es1ExpectedValue != nil {
			assert.Equal(t, *step.es1ExpectedValue, es1FakeClient.excludedNodesFromShardAllocation)
		}
		if step.es2ExpectedValue != nil {
			assert.Equal(t, *step.es2ExpectedValue, es2FakeClient.excludedNodesFromShardAllocation)
		}
	}
}

func intPtr(value int) *int {
	return &value
}

func Test_cachedClient_SetMinimumMasterNodes(t *testing.T) {
	cacheClientBuilder := NewCachedClientBuilder()

	es1FakeClient := fakeESClient{}
	es1FakeCachedClient := cacheClientBuilder.NewElasticsearchCachedClient(es1, &es1FakeClient)

	es2FakeClient := fakeESClient{}
	es2FakeCachedClient := cacheClientBuilder.NewElasticsearchCachedClient(es2, &es2FakeClient)

	steps := []struct {
		es1Value, es2Value                   *int
		es1ExpectedValue, es2ExpectedValue   *int
		es1ExpectedCalled, es2ExpectedCalled int
		es1Error, es2Error                   error
	}{
		{
			es1Value:          intPtr(1),
			es1ExpectedValue:  intPtr(1),
			es1ExpectedCalled: 1,
			es2ExpectedCalled: 0,
		},
		{
			es1Value:          intPtr(1),
			es1ExpectedValue:  intPtr(1),
			es2Value:          intPtr(2),
			es2ExpectedValue:  intPtr(2),
			es1ExpectedCalled: 1, // same value, es1 is not called
			es2ExpectedCalled: 1,
		},
		{
			es1Value:          intPtr(3),
			es1ExpectedValue:  intPtr(3),
			es1ExpectedCalled: 2,
			es2ExpectedCalled: 1,
		},
		{
			es1Value:          intPtr(1),
			es1Error:          fmt.Errorf("error"),
			es2ExpectedValue:  intPtr(2),
			es1ExpectedCalled: 3,
			es2ExpectedCalled: 1,
		},
		{
			es1Value:          intPtr(3),
			es1ExpectedValue:  intPtr(3),
			es2ExpectedValue:  intPtr(2), // es2 value should still be in memory
			es1ExpectedCalled: 4,         // API must be called after the error at the previous step
			es2ExpectedCalled: 1,
		},
	}

	for _, step := range steps {
		// Set error field in fake client
		es1FakeClient.err = step.es1Error
		es2FakeClient.err = step.es2Error

		// Simulate API calls
		if step.es1Value != nil {
			_ = es1FakeCachedClient.SetMinimumMasterNodes(nil, *step.es1Value)
		}
		if step.es2Value != nil {
			_ = es2FakeCachedClient.SetMinimumMasterNodes(nil, *step.es2Value)
		}
		assert.Equal(t, step.es1ExpectedCalled, es1FakeClient.setMinimumMasterNodesCalled)
		assert.Equal(t, step.es2ExpectedCalled, es2FakeClient.setMinimumMasterNodesCalled)
		if step.es1ExpectedValue != nil {
			assert.Equal(t, *step.es1ExpectedValue, es1FakeClient.minimumMasterNodes)
		}
		if step.es2ExpectedValue != nil {
			assert.Equal(t, *step.es2ExpectedValue, es2FakeClient.minimumMasterNodes)
		}
	}
}
