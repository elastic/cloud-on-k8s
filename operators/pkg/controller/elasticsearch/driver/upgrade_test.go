// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	nodeNames:               nil,
}

type mockUpdater map[string]int32

func (m mockUpdater) updatePartition(sset *appsv1.StatefulSet, newPartition int32) error {
	m[sset.Name] = newPartition
	return nil
}

type mockPodCheck map[string]bool

func (m mockPodCheck) podUpgradeDone(_ k8s.Client, _ ESState, nsn types.NamespacedName, _ string) (bool, error) {
	return m[nsn.Name], nil
}

func success() *reconciler.Results {
	return &reconciler.Results{}
}

func Test_defaultDriver_doRollingUpgrade(t *testing.T) {
	// This does not test: podUpgradeDone or prepareClusterForNodeRestart in detail but is focused on the main invariants
	// of the run method
	type args struct {
		statefulSets sset.StatefulSetList
		esState      ESState
	}
	tests := []struct {
		name             string
		args             args
		upgradedPods     map[string]bool
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
			want: success(),
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
			want: success().WithResult(defaultRequeue),
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
						Replicas: 3,
						Master:   true,
					}.Build(),
					nodespec.TestSset{
						Name:      "data",
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
			want: success(),
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
			want:             success().WithResult(defaultRequeue),
			wantNewPartition: map[string]int32{},
		},
		{
			name: "partially rolled out upgrade",
			args: args{
				statefulSets: sset.StatefulSetList{
					nodespec.TestSset{
						Name:      "default",
						Replicas:  2,
						Data:      true,
						Partition: 1, // one node has been upgraded
						Status: appsv1.StatefulSetStatus{
							CurrentRevision: "a",
							UpdateRevision:  "b",
						},
					}.Build(),
				},
				esState: defaultESState,
			},
			upgradedPods: map[string]bool{
				"default-1": true, // its pod is ready
			},
			want: success(),
			wantNewPartition: map[string]int32{
				"default": 0, // expect now the remaining node to be rolled
			},
			wantSyncedFlush: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			for i := range tt.args.statefulSets {
				runtimeObjects = append(runtimeObjects, &tt.args.statefulSets[i])
			}
			k8sClient := k8s.WrapClient(fake.NewFakeClient(runtimeObjects...))
			mu := mockUpdater{}
			fc := fakeESClient{}
			upgrade := rollingUpgradeCtx{
				client:         k8sClient,
				ES:             v1alpha1.Elasticsearch{},
				statefulSets:   tt.args.statefulSets,
				esClient:       &fc,
				esState:        tt.args.esState,
				podUpgradeDone: mockPodCheck(tt.upgradedPods).podUpgradeDone,
				upgrader:       mu.updatePartition,
			}
			if got := upgrade.run(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("run() = %+v, want %+v", got, tt.want)
			}
			assert.Nil(t, deep.Equal(map[string]int32(mu), tt.wantNewPartition))
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
			want:           success().WithResult(defaultRequeue),
		},
		{
			name: "update done but node not in cluster",
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
			want: success().WithResult(defaultRequeue),
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
			want:                  success(),
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

func Test_podUpgradeDone(t *testing.T) {
	type args struct {
		esState          ESState
		podRef           types.NamespacedName
		expectedRevision string
	}
	const testPod = "test"
	const upgradeRevision = "rev1"
	const testNamespace = "default"
	nsn := types.NamespacedName{Name: testPod, Namespace: testNamespace}
	defaultArgs := args{
		podRef:           nsn,
		expectedRevision: upgradeRevision,
	}
	currentTime := metav1.NewTime(time.Now())
	tests := []struct {
		name           string
		runtimeObjects []runtime.Object
		args           args
		want           bool
		wantErr        bool
	}{
		{
			name: "pod not found: not done",
			args: args{
				podRef:           nsn,
				expectedRevision: upgradeRevision,
			},
			want: false,
		},
		{
			name: "pod deleting: not done",
			runtimeObjects: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              testPod,
						DeletionTimestamp: &currentTime,
					},
				},
			},
			args: defaultArgs,
			want: false,
		},
		{
			name: "pod on incorrect rev: not done",
			runtimeObjects: []runtime.Object{
				nodespec.TestPod{
					Namespace: testNamespace,
					Name:      testPod,
					Revision:  "rev0",
				}.BuildPtr(),
			},
			args: defaultArgs,
			want: false,
		},
		{
			name: "pod not ready: not done",
			runtimeObjects: []runtime.Object{
				nodespec.TestPod{
					Namespace: testNamespace,
					Name:      testPod,
					Revision:  upgradeRevision,
				}.BuildPtr(),
			},
			args: defaultArgs,
			want: false,
		},
		{
			name: "ES node not in cluster: not done",
			runtimeObjects: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      testPod,
						Labels: map[string]string{
							appsv1.StatefulSetRevisionLabel: upgradeRevision,
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			args: args{
				esState:          mockESState{},
				podRef:           nsn,
				expectedRevision: upgradeRevision,
			},
			want: false,
		},
		{
			name: "pod upgraded",
			runtimeObjects: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      testPod,
						Labels: map[string]string{
							appsv1.StatefulSetRevisionLabel: upgradeRevision,
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			args: args{
				esState: mockESState{
					nodeNames: []string{testPod},
				},
				podRef:           nsn,
				expectedRevision: upgradeRevision,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrapClient(fake.NewFakeClient(tt.runtimeObjects...))
			got, err := podUpgradeDone(k8sClient, tt.args.esState, tt.args.podRef, tt.args.expectedRevision)
			if (err != nil) != tt.wantErr {
				t.Errorf("podUpgradeDone() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("podUpgradeDone() got = %v, want %v", got, tt.want)
			}
		})
	}
}
