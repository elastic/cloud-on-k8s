// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMigratingData(t *testing.T) {
	type args struct {
		shardLister client.ShardLister
		podName     string
		exclusions  []string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Test enough redundancy",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "B"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "C"},
				}),
			},
			want: false,
		},
		{
			name: "Nothing to migrate",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "B"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "C"},
				}),
			},
			want: false,
		},
		{
			name: "Only copy needs migration",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
					{Index: "index-1", Shard: "1", State: client.STARTED, Node: "B"},
					{Index: "index-1", Shard: "2", State: client.STARTED, Node: "C"},
				}),
			},
			want: true,
		},
		{
			name: "Relocation is migration",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.RELOCATING, Node: "A"},
					{Index: "index-1", Shard: "0", State: client.INITIALIZING, Node: "B"},
				}),
			},
			want: true,
		},
		{
			name: "Copy is initializing",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
					{Index: "index-1", Shard: "0", State: client.INITIALIZING, Node: "B"},
				}),
			},
			want: true,
		},
		{
			name: "Valid copy exists",
			args: args{
				podName:    "A",
				exclusions: []string{"A", "B"},
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "B"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "C"},
				}),
			},
			want: false,
		},
		{
			name: "No Valid copy exists, all nodes are excluded",
			args: args{
				podName:    "A",
				exclusions: []string{"B", "C"},
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "A"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "B"},
					{Index: "index-1", Shard: "0", State: client.STARTED, Node: "C"},
				}),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsMigratingData(tt.args.shardLister, tt.args.podName, tt.args.exclusions)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsMigratingData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsMigratingData() = %v, want %v", got, tt.want)
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
			name:         "no nodes to migrate",
			leavingNodes: []string{},
			want:         "none_excluded",
		},
		{
			name:         "one node to migrate",
			leavingNodes: []string{"test-node"},
			want:         "test-node",
		},
		{
			name:         "multiple node to migrate",
			leavingNodes: []string{"test-node1", "test-node2"},
			want:         "test-node1,test-node2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocationSetter := fakeAllocationSetter{}
			err := MigrateData(&allocationSetter, tt.leavingNodes)
			require.NoError(t, err)
			assert.Contains(t, allocationSetter.value, tt.want)
		})
	}
}
