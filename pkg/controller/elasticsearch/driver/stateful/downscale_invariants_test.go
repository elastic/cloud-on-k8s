// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

func Test_newDownscaleState(t *testing.T) {
	es := esv1.Elasticsearch{
		Namespace: ssetMaster3Replicas.Namespace, Name: "name",
		Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 4}}},
	}
	testPod := func(name string, master, ready bool) corev1.Pod {
		return sset.TestPod{Namespace: es.Namespace, Name: name, ClusterName: es.Name, StatefulSetName: "sset", Master: master, Ready: ready}.Build()
	}
	terminating := func(pod corev1.Pod) corev1.Pod {
		pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		return pod
	}
	// master-0 and data-0 are ready, the other Pods are not ready and data-2 is terminating
	mixedPods := []corev1.Pod{
		testPod("master-0", true, true),
		testPod("master-1", true, false),
		testPod("data-0", false, true),
		testPod("data-1", false, false),
		terminating(testPod("data-2", false, false)),
		testPod("data-3", false, false),
	}

	masterSset := func(replicas int32) es_sset.StatefulSetList {
		return es_sset.StatefulSetList{
			sset.TestSset{Namespace: es.Namespace, Name: "master", ClusterName: es.Name, Master: true, Replicas: replicas}.Build(),
			sset.TestSset{Namespace: es.Namespace, Name: "data", ClusterName: es.Name, Data: true, Replicas: 4}.Build(),
		}
	}

	tests := []struct {
		name               string
		actualPods         []corev1.Pod
		actualStatefulSets es_sset.StatefulSetList
		drainingNodes      set.StringSet
		want               *downscaleState
	}{
		{
			name:       "no resources in the apiserver",
			actualPods: nil,
			want:       &downscaleState{masterRemovalInProgress: false, runningMasters: 0, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1))},
		},
		{
			name: "3 masters running in the apiserver, 1 not running",
			actualPods: []corev1.Pod{
				// 3 masters running
				{
					Namespace: ssetMaster3Replicas.Namespace,
					Name:      ssetMaster3Replicas.Name + "-0",
					Labels: map[string]string{
						label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
						string(label.NodeTypesMasterLabelName): "true",
						label.ClusterNameLabelName:             es.Name,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Namespace: ssetMaster3Replicas.Namespace,
					Name:      ssetMaster3Replicas.Name + "-1",
					Labels: map[string]string{
						label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
						string(label.NodeTypesMasterLabelName): "true",
						label.ClusterNameLabelName:             es.Name,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Namespace: ssetMaster3Replicas.Namespace,
					Name:      ssetMaster3Replicas.Name + "-2",
					Labels: map[string]string{
						label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
						string(label.NodeTypesMasterLabelName): "true",
						label.ClusterNameLabelName:             es.Name,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				// 1 master not ready yet
				{
					Namespace: ssetMaster3Replicas.Namespace,
					Name:      ssetMaster3Replicas.Name + "-3",
					Labels: map[string]string{
						label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
						string(label.NodeTypesMasterLabelName): "true",
						label.ClusterNameLabelName:             es.Name,
					},
				},
			},
			want: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1))},
		},
		{
			name:       "no draining nodes",
			actualPods: mixedPods,
			want:       &downscaleState{masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1))},
		},
		{
			name:               "draining nodes are not available but tracked unless ready or terminating",
			actualPods:         mixedPods,
			actualStatefulSets: masterSset(2),
			drainingNodes:      set.Make("master-1", "data-0", "data-1", "data-2"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				drainingNodes: set.Make("master-1", "data-1"), drainingMasters: set.Make("master-1"),
			},
		},
		{
			name:               "a draining master that is not the next to be removed is not a removal in progress",
			actualPods:         mixedPods,
			actualStatefulSets: masterSset(3),
			drainingNodes:      set.Make("master-1"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				drainingNodes: set.Make("master-1"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDownscaleState(tt.actualPods, tt.actualStatefulSets, es, tt.drainingNodes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewDownscaleInvariants() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_calculateRemovalsAllowed(t *testing.T) {
	tests := []struct {
		name           string
		nodesReady     int32
		desiredNodes   int32
		maxUnavailable *int32
		want           *int32
	}{
		{
			name:           "default should be 1",
			nodesReady:     5,
			desiredNodes:   5,
			maxUnavailable: nil,
			want:           nil,
		},
		{
			name:           "scaling down, at least one node up",
			nodesReady:     10,
			desiredNodes:   3,
			maxUnavailable: new(int32(2)),
			want:           new(int32(9)),
		},
		{
			name:           "scaling up, can't remove anything",
			nodesReady:     3,
			desiredNodes:   5,
			maxUnavailable: new(int32(1)),
			want:           new(int32(0)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRemovalsAllowed(tt.nodesReady, tt.desiredNodes, tt.maxUnavailable)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculateRemovalsAllowed() got = %v, want = %v", got, tt.want)
			}
		})
	}
}

func Test_checkDownscaleInvariants(t *testing.T) {
	tests := []struct {
		name             string
		state            *downscaleState
		statefulSet      appsv1.StatefulSet
		wantCanDownscale bool
		wantReason       string
	}{
		{
			name:             "should allow removing data node if maxUnavailable allows",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: new(int32(1))},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing data nodes of maxUnavailable disallows",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: new(int32(0))},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name:             "should allow removing one master if there is another one running",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1))},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing the last master",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(1))},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name:             "should not allow removing a master if one is already being removed",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: true, removalsAllowed: new(int32(2))},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant,
		},
		{
			name: "should allow removing a draining data node if maxUnavailable disallows",
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)),
				drainingNodes: set.Make(sset.PodName(ssetData4Replicas.Name, 3)),
			},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name: "should allow removing a draining master if it is not the last running one",
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)),
				drainingNodes: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name: "should not allow removing a master if another master is draining",
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1)),
				drainingMasters: set.Make("other-master-0"),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant,
		},
		{
			name: "should allow removing a draining master if another master is draining",
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1)),
				drainingMasters: set.Make("other-master-0", sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name: "should allow removing a data node if a master is draining",
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1)),
				drainingMasters: set.Make("other-master-0"),
			},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing a master if maxUnavailable disallows",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0))},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toDelete, reason := checkDownscaleInvariants(*tt.state, tt.statefulSet, 1)
			canDownscale := toDelete == 1
			if canDownscale != tt.wantCanDownscale {
				t.Errorf("canDownscale() canDownscale = %v, want %v", canDownscale, tt.wantCanDownscale)
			}
			if reason != tt.wantReason {
				t.Errorf("canDownscale() reason = %v, want %v", reason, tt.wantReason)
			}
		})
	}
}

func Test_downscaleState_recordRemoval(t *testing.T) {
	tests := []struct {
		name        string
		statefulSet appsv1.StatefulSet
		removals    int32
		state       *downscaleState
		wantState   *downscaleState
	}{
		{
			name:        "removing a data node should decrease nodes available for removal",
			statefulSet: ssetData4Replicas,
			removals:    1,
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1))},
			wantState:   &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0))},
		},
		{
			name:        "removing many data nodes should decrease nodes available for removal",
			statefulSet: ssetData4Replicas,
			removals:    3,
			state:       &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(3))},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0))},
		},
		{
			name:        "removing a master node should mutate the budget",
			statefulSet: ssetMaster3Replicas,
			removals:    1,
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(2))},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: new(int32(1))},
		},
		{
			name:        "removing a draining master node should not mutate the budget",
			statefulSet: ssetMaster3Replicas,
			removals:    1,
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)),
				drainingNodes: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			wantState: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: new(int32(0)),
				drainingNodes: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
		},
		{
			name:        "removing draining and ready data nodes should only count the ready ones in the budget",
			statefulSet: ssetData4Replicas,
			removals:    2,
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(1)),
				drainingNodes: set.Make(sset.PodName(ssetData4Replicas.Name, 3)), drainingRemovalsAllowed: new(int32(1)),
			},
			wantState: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)),
				drainingNodes: set.Make(sset.PodName(ssetData4Replicas.Name, 3)), drainingRemovalsAllowed: new(int32(0)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.state.recordNodeRemoval(tt.statefulSet, tt.removals)
			require.Equal(t, tt.wantState, tt.state)
		})
	}
}

func Test_downscaleState_getMaxNodesToRemove(t *testing.T) {
	podName := func(ordinal int32) string {
		return sset.PodName(ssetData4Replicas.Name, ordinal)
	}
	tests := []struct {
		name                    string
		removalsAllowed         *int32
		drainingRemovalsAllowed *int32
		drainingNodes           set.StringSet
		noMoreThan              int32
		want                    int32
	}{
		{
			name:       "no budget",
			noMoreThan: 3,
			want:       3,
		},
		{
			name:            "budget limits the removals",
			removalsAllowed: new(int32(2)),
			noMoreThan:      3,
			want:            2,
		},
		{
			name:            "draining nodes do not consume the budget",
			removalsAllowed: new(int32(0)),
			drainingNodes:   set.Make(podName(3), podName(2)),
			noMoreThan:      3,
			want:            2,
		},
		{
			name:            "draining nodes and ready nodes share the removals",
			removalsAllowed: new(int32(1)),
			drainingNodes:   set.Make(podName(2)),
			noMoreThan:      3,
			want:            2,
		},
		{
			name:            "a draining node behind a ready node needs budget for the ready node",
			removalsAllowed: new(int32(0)),
			drainingNodes:   set.Make(podName(2)),
			noMoreThan:      3,
			want:            0,
		},
		{
			name:                    "draining removals are limited",
			removalsAllowed:         new(int32(0)),
			drainingRemovalsAllowed: new(int32(1)),
			drainingNodes:           set.Make(podName(3), podName(2)),
			noMoreThan:              3,
			want:                    1,
		},
		{
			name:                    "draining removals and budget are limited independently",
			removalsAllowed:         new(int32(1)),
			drainingRemovalsAllowed: new(int32(1)),
			drainingNodes:           set.Make(podName(3), podName(1)),
			noMoreThan:              3,
			want:                    2,
		},
		{
			name:            "draining nodes of another StatefulSet do not allow removals",
			removalsAllowed: new(int32(0)),
			drainingNodes:   set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			noMoreThan:      3,
			want:            0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := downscaleState{removalsAllowed: tt.removalsAllowed, drainingRemovalsAllowed: tt.drainingRemovalsAllowed, drainingNodes: tt.drainingNodes}
			require.Equal(t, tt.want, state.getMaxNodesToRemove(ssetData4Replicas, tt.noMoreThan))
		})
	}
}

func Test_calculateDrainingRemovalsAllowed(t *testing.T) {
	tests := []struct {
		name           string
		maxUnavailable *int32
		want           *int32
	}{
		{
			name: "no limit",
			want: nil,
		},
		{
			name:           "maxUnavailable",
			maxUnavailable: new(int32(2)),
			want:           new(int32(2)),
		},
		{
			name:           "at least one when maxUnavailable is 0",
			maxUnavailable: new(int32(0)),
			want:           new(int32(1)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, calculateDrainingRemovalsAllowed(tt.maxUnavailable))
		})
	}
}
