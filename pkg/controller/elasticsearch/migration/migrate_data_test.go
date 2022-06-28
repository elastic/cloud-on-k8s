// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package migration

import (
	"context"
	"fmt"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeMayHaveShard(t *testing.T) {
	type args struct {
		shardLister client.ShardLister
		podName     string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Error while getting shards",
			args: args{
				podName: "A",
				shardLister: NewFakeShardListerWithError(
					[]client.Shard{},
					fmt.Errorf("error")),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Node has one shard",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", NodeName: "A"},
					{Index: "index-1", Shard: "0", NodeName: "B"},
					{Index: "index-1", Shard: "0", NodeName: "C"},
				}),
			},
			want: true,
		},
		{
			name: "No shard on the node",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", NodeName: "B"},
					{Index: "index-1", Shard: "0", NodeName: "C"},
				}),
			},
			want: false,
		},
		{
			name: "Some shards have no node assigned",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", NodeName: ""},
					{Index: "index-1", Shard: "0", NodeName: "C"},
				}),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nodeMayHaveShard(context.Background(), esv1.Elasticsearch{}, tt.args.shardLister, tt.args.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("nodeMayHaveShard() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("nodeMayHaveShard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMigrateData(t *testing.T) {
	tests := []struct {
		name         string
		leavingNodes []string
		want         string
	}{
		{
			name:         "no nodes to migrate, allocation setting should be set to none_excluded",
			leavingNodes: []string{},
			want:         "none_excluded",
		},
		{
			name:         "a node to migrate",
			leavingNodes: []string{"test-node1"},
			want:         "test-node1",
		},
		{
			name:         "multiple nodes to migrate",
			leavingNodes: []string{"test-node1", "test-node2"},
			want:         "test-node1,test-node2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocationSetter := fakeAllocationSetter{}
			err := migrateData(context.Background(), esv1.Elasticsearch{}, &allocationSetter, tt.leavingNodes)
			require.NoError(t, err)
			assert.Equal(t, tt.want, allocationSetter.value)
		})
	}
}
