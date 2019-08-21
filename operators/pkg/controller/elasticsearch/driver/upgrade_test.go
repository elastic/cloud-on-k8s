// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockESState struct {
	shardAllocationsEnabled bool
	green                   bool
	nodeNames               []string
}

func (m mockESState) NodesInCluster(nodeNames []string) (bool, error) {
	return stringsutil.StringsInSlice(nodeNames, m.nodeNames), nil
}

func (m mockESState) ShardAllocationsEnabled() (bool, error) {
	return m.shardAllocationsEnabled, nil
}

func (m mockESState) GreenHealth() (bool, error) {
	return m.green, nil
}

var _ ESState = mockESState{}

var defaultESState = mockESState{
	shardAllocationsEnabled: false,
	green:                   true,
	nodeNames:               []string{"default-0"},
}

type mockUpdater map[string]int32

func (m mockUpdater) updatePartition(sset *appsv1.StatefulSet, newPartition int32) error {
	m[sset.Name] = newPartition
	return nil
}

func newResults() *reconciler.Results {
	return &reconciler.Results{}
}

func Test_defaultDriver_doRollingUpgrade(t *testing.T) {
	// This does not test: podUpgradeDone or prepareClusterForNodeRestart in detail but is focused on the main invariants
	// of the doRollingUpgrade method
	type args struct {
		statefulSets sset.StatefulSetList
		esState      ESState
	}
	tests := []struct {
		name             string
		args             args
		want             *reconciler.Results
		wantNewPartition map[string]int32
		wantSyncedFlush  bool
	}{
		{
			name: "single sset upgrade",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:      "default",
						Version:   "7.3.0",
						Replicas:  1,
						Master:    true,
						Data:      true,
						Partition: 1,
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: defaultESState,
			},
			want: newResults(),
			wantNewPartition: map[string]int32{
				"default": 0,
			},
			wantSyncedFlush: true,
		},
		{
			name: "just one (master) at a time",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:      "default",
						Version:   "7.3.0",
						Replicas:  3,
						Master:    true,
						Data:      true,
						Partition: 3,
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: defaultESState,
			},
			want: newResults().WithResult(defaultRequeue),
			wantNewPartition: map[string]int32{
				"default": 2,
			},
			wantSyncedFlush: true,
		},
		{
			name: "multiple ssets, update correct sset",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:     "master",
						Version:  "7.3.0",
						Replicas: 3,
						Master:   true,
					}.Build(),
					nodespec.TestSset{
						Name:      "data",
						Version:   "7.3.0",
						Replicas:  1,
						Data:      true,
						Partition: 1,
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: defaultESState,
			},
			want: newResults(),
			wantNewPartition: map[string]int32{
				"data": 0,
			},
			wantSyncedFlush: true,
		},
		{
			name: "wait for healthy cluster",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:      "default",
						Version:   "7.3.0",
						Replicas:  1,
						Master:    true,
						Data:      true,
						Partition: 2,
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: mockESState{
					green: false,
				},
			},
			want:             newResults().WithResult(defaultRequeue),
			wantNewPartition: map[string]int32{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var runtimeObjects []runtime.Object
			for i := range tt.args.statefulSets {
				runtimeObjects = append(runtimeObjects, &tt.args.statefulSets[i])
			}

			k8sClient := k8s.WrapClient(fake.NewFakeClient(runtimeObjects...))

			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					Client: k8sClient,
					Scheme: scheme.Scheme,
				},
			}
			mu := mockUpdater{}
			fc := fakeESClient{}
			if got := d.doRollingUpgrade(tt.args.statefulSets, &fc, tt.args.esState, mu.updatePartition); !reflect.DeepEqual(got, tt.want) {
				result, err := got.Aggregate()
				wantRes, wantErr := tt.want.Aggregate()
				t.Errorf("doRollingUpgrade() = %v, want %v,err = %v, want %v", result, wantRes, err, wantErr)
			}
			if diff := deep.Equal(map[string]int32(mu), tt.wantNewPartition); diff != nil {
				t.Error(diff)
			}
			require.Equal(t, tt.wantSyncedFlush, fc.SyncedFlushCalled, "Synced Flush API call")
		})
	}
}

func Test_defaultDriver_MaybeEnableShardsAllocation(t *testing.T) {
	type args struct {
		esState      ESState
		statefulSets sset.StatefulSetList
	}
	tests := []struct {
		name                  string
		args                  args
		runtimeObjects        []runtime.Object
		want                  *reconciler.Results
		wantAllocationEnabled bool
	}{
		{
			name: "still update pending",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:      "default",
						Version:   "7.3.0",
						Replicas:  1,
						Master:    true,
						Data:      true,
						Partition: 0, // upgrade rolled out
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: defaultESState,
			},
			runtimeObjects: nil, // but no corresponding pod in runtime objects
			want:           newResults().WithResult(defaultRequeue),
		},
		{
			name: "update done but node not in cluster",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name: "default",
						//	Version:   "7.3.0",
						Replicas:  1,
						Master:    true,
						Data:      true,
						Partition: 0, // upgrade rolled out
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: mockESState{
					shardAllocationsEnabled: false,
					green:                   false,
					nodeNames:               nil, // no node here
				},
			},
			runtimeObjects: []runtime.Object{
				nodespec.TestPod{
					Name:     "default-0",
					Revision: "b", // pod at latest revision
					Master:   true,
					Data:     true,
				}.BuildPtr(),
			},
			want: newResults().WithResult(defaultRequeue),
		},
		{
			name: "should enable shard allocations",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:      "default",
						Replicas:  1,
						Master:    true,
						Data:      true,
						Partition: 0, // upgrade rolled out
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: defaultESState, // default state contains pod 0
			},
			runtimeObjects: []runtime.Object{
				nodespec.TestPod{
					Name:     "default-0",
					Revision: "b", // pod at latest revision
					Master:   true,
					Data:     true,
				}.BuildPtr(),
			},
			want:                  newResults(),
			wantAllocationEnabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.args.statefulSets {
				tt.runtimeObjects = append(tt.runtimeObjects, &tt.args.statefulSets[i])
			}

			k8sClient := k8s.WrapClient(fake.NewFakeClient(tt.runtimeObjects...))

			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					Client: k8sClient,
					Scheme: scheme.Scheme,
				},
			}
			c := fakeESClient{}
			if got := d.MaybeEnableShardsAllocation(&c, tt.args.esState, tt.args.statefulSets); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MaybeEnableShardsAllocation() = %v, want %v", got, tt.want)
			}
			require.Equal(t, tt.wantAllocationEnabled, c.EnableShardAllocationCalled, "shard allocation enabled API called")
		})
	}
}
