// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMigratingData(t *testing.T) {
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
			name: "Node needs to be completely evacuated",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "A"},
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "B"},
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "C"},
				}),
			},
			want: true,
		},
		{
			name: "Nothing to migrate",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "B"},
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "C"},
				}),
			},
			want: false,
		},
		{
			name: "Only copy needs migration",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "A"},
					{Index: "index-1", Shard: "1", State: client.STARTED, NodeName: "B"},
					{Index: "index-1", Shard: "2", State: client.STARTED, NodeName: "C"},
				}),
			},
			want: true,
		},
		{
			name: "Relocation is migration",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.RELOCATING, NodeName: "A"},
					{Index: "index-1", Shard: "0", State: client.INITIALIZING, NodeName: "B"},
				}),
			},
			want: true,
		},
		{
			name: "Copy is initializing",
			args: args{
				podName: "A",
				shardLister: NewFakeShardLister([]client.Shard{
					{Index: "index-1", Shard: "0", State: client.STARTED, NodeName: "A"},
					{Index: "index-1", Shard: "0", State: client.INITIALIZING, NodeName: "B"},
				}),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsMigratingData(context.Background(), tt.args.shardLister, tt.args.podName)
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
		es           esv1.Elasticsearch
		leavingNodes []string
		want         string
		wantEs       esv1.Elasticsearch
	}{
		{
			name:         "no nodes to migrate, no annotation on ES",
			es:           esv1.Elasticsearch{},
			leavingNodes: []string{},
			want:         "none_excluded",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "none_excluded"},
			}},
		},
		{
			name: "no nodes to migrate, annotation already set on ES",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "none_excluded"},
			}},
			leavingNodes: []string{},
			want:         "",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "none_excluded"},
			}},
		},
		{
			name: "no nodes to migrate, annotation set with some exclusions on ES",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node1,test-node2"},
			}},
			leavingNodes: []string{},
			want:         "none_excluded",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "none_excluded"},
			}},
		},
		{
			name:         "one node to migrate, no annotation set on ES",
			es:           esv1.Elasticsearch{},
			leavingNodes: []string{"test-node"},
			want:         "test-node",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node"},
			}},
		},
		{
			name: "one node to migrate, no exclusions in ES annotation",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "none_excluded"},
			}},
			leavingNodes: []string{"test-node"},
			want:         "test-node",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node"},
			}},
		},
		{
			name: "one node to migrate, different exclusions in ES annotation",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node2"},
			}},
			leavingNodes: []string{"test-node"},
			want:         "test-node",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node"},
			}},
		},
		{
			name: "one node to migrate, already present in ES annotation",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node"},
			}},
			leavingNodes: []string{"test-node"},
			want:         "",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node"},
			}},
		},
		{
			name: "multiple node to migrate, no exclusions in ES annotation",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "none_excluded"},
			}},
			leavingNodes: []string{"test-node1", "test-node2"},
			want:         "test-node1,test-node2",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node1,test-node2"},
			}},
		},
		{
			name: "multiple node to migrate, different exclusions in ES annotation",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node1,test-node3"},
			}},
			leavingNodes: []string{"test-node1", "test-node2"},
			want:         "test-node1,test-node2",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node1,test-node2"},
			}},
		},
		{
			name: "multiple node to migrate, already present in ES annotation",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node1,test-node2"},
			}},
			leavingNodes: []string{"test-node1", "test-node2"},
			want:         "",
			wantEs: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{AllocationExcludeAnnotationName: "test-node1,test-node2"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocationSetter := fakeAllocationSetter{}
			c := k8s.WrappedFakeClient(&tt.es)
			err := MigrateData(context.Background(), c, tt.es, &allocationSetter, tt.leavingNodes)
			require.NoError(t, err)
			assert.Contains(t, allocationSetter.value, tt.want)
			var retrievedES esv1.Elasticsearch
			err = c.Get(k8s.ExtractNamespacedName(&tt.es), &retrievedES)
			require.NoError(t, err)
			require.Equal(t, tt.wantEs.Annotations, retrievedES.Annotations)
		})
	}
}
