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
		nodesWithShutdown  set.StringSet
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
			want: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)), unavailableMasters: set.Make(ssetMaster3Replicas.Name + "-3")},
		},
		{
			name:       "a terminating master is unavailable and not running",
			actualPods: []corev1.Pod{testPod("master-0", true, true), terminating(testPod("master-1", true, true))},
			want:       &downscaleState{masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)), unavailableMasters: set.Make("master-1")},
		},
		{
			name:       "no draining nodes",
			actualPods: mixedPods,
			want:       &downscaleState{masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)), unavailableMasters: set.Make("master-1")},
		},
		{
			name:               "draining nodes are not available but tracked unless ready or terminating",
			actualPods:         mixedPods,
			actualStatefulSets: masterSset(2),
			nodesWithShutdown:  set.Make("master-1", "data-0", "data-1", "data-2"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make("master-1"), unavailableDrainingPods: set.Make("master-1", "data-1"), drainingMasters: set.Make("master-1"),
			},
		},
		{
			name:               "a draining master that is not the next to be removed is not a removal in progress",
			actualPods:         mixedPods,
			actualStatefulSets: masterSset(3),
			nodesWithShutdown:  set.Make("master-1"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make("master-1", "master-2"), unavailableDrainingPods: set.Make("master-1"),
			},
		},
		{
			name: "only the draining masters that are the next to be removed of each master StatefulSet are removals in progress",
			actualPods: []corev1.Pod{
				testPod("a-0", true, true), testPod("a-1", true, true),
				testPod("b-0", true, false), testPod("b-1", true, false),
			},
			actualStatefulSets: es_sset.StatefulSetList{
				sset.TestSset{Namespace: es.Namespace, Name: "a", ClusterName: es.Name, Master: true, Replicas: 2}.Build(),
				sset.TestSset{Namespace: es.Namespace, Name: "b", ClusterName: es.Name, Master: true, Replicas: 2}.Build(),
			},
			// b-0 is draining but b-1 is the next to be removed from b
			nodesWithShutdown: set.Make("b-0", "b-1"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 2, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make("b-0", "b-1"), unavailableDrainingPods: set.Make("b-0", "b-1"), drainingMasters: set.Make("b-1"),
			},
		},
		{
			name:               "a missing master is unavailable and not running",
			actualPods:         []corev1.Pod{testPod("master-0", true, true), testPod("master-2", true, false)},
			actualStatefulSets: masterSset(3),
			nodesWithShutdown:  set.Make("master-2"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make("master-1", "master-2"), unavailableDrainingPods: set.Make("master-2"), drainingMasters: set.Make("master-2"),
			},
		},
		{
			name:               "a terminating draining master is unavailable and a removal in progress, but not free to remove",
			actualPods:         []corev1.Pod{testPod("master-0", true, true), terminating(testPod("master-1", true, false))},
			actualStatefulSets: masterSset(2),
			nodesWithShutdown:  set.Make("master-1"),
			want: &downscaleState{
				masterRemovalInProgress: false, runningMasters: 1, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make("master-1"), drainingMasters: set.Make("master-1"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDownscaleState(tt.actualPods, tt.actualStatefulSets, es, tt.nodesWithShutdown, nil)
			// the sets omitted from the expected state are empty
			for _, s := range []*set.StringSet{&tt.want.unavailableDrainingPods, &tt.want.unavailableMasters, &tt.want.drainingMasters} {
				if *s == nil {
					*s = set.Make()
				}
			}
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
	dataPod := func(ordinal int32, ready bool) corev1.Pod {
		return sset.TestPod{
			Namespace: ssetData4Replicas.Namespace, Name: sset.PodName(ssetData4Replicas.Name, ordinal),
			StatefulSetName: ssetData4Replicas.Name, ClusterName: clusterName, Data: true, Ready: ready,
		}.Build()
	}
	// the last data Pod is draining and terminating, and another one is not ready: with 3 desired nodes and maxUnavailable
	// 1, the budget does not allow any removal
	terminatingDrainingPod := dataPod(3, false)
	terminatingDrainingPod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	terminatingDrainingDataState := newDownscaleState(
		[]corev1.Pod{dataPod(0, true), dataPod(1, true), dataPod(2, false), terminatingDrainingPod},
		es_sset.StatefulSetList{ssetData4Replicas},
		esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 3}}}},
		set.Make(terminatingDrainingPod.Name),
		nil,
	)
	masterPod := func(ordinal int32, ready bool) corev1.Pod {
		return sset.TestPod{
			Namespace: ssetMaster3Replicas.Namespace, Name: sset.PodName(ssetMaster3Replicas.Name, ordinal),
			StatefulSetName: ssetMaster3Replicas.Name, ClusterName: clusterName, Master: true, Ready: ready,
		}.Build()
	}
	// the last master is draining, the only other ready master is terminating: with 1 desired node and maxUnavailable 1,
	// the budget allows the removal
	terminatingMasterPod := masterPod(1, true)
	terminatingMasterPod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	drainingMasterWithTerminatingMasterState := newDownscaleState(
		[]corev1.Pod{masterPod(0, false), terminatingMasterPod, masterPod(2, false)},
		es_sset.StatefulSetList{ssetMaster3Replicas},
		esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 1}}}},
		set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
		nil,
	)
	// the last master is draining and the Pod of another master is missing: with 2 desired nodes and maxUnavailable 1, the
	// budget does not allow the removal
	drainingMasterWithMissingMasterState := newDownscaleState(
		[]corev1.Pod{masterPod(0, true), masterPod(2, false)},
		es_sset.StatefulSetList{ssetMaster3Replicas},
		esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 2}}}},
		set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
		nil,
	)
	// the StatefulSet "changed" already has a data-only template, but its Pods still run as masters, and its last one is
	// draining: with 3 desired nodes and maxUnavailable 1, the budget does not allow any removal
	ssetChangedToData := sset.TestSset{Namespace: ssetMaster3Replicas.Namespace, Name: "changed", ClusterName: clusterName, Data: true, Replicas: 2}.Build()
	ssetOtherMaster := sset.TestSset{Namespace: ssetMaster3Replicas.Namespace, Name: "other", ClusterName: clusterName, Master: true, Replicas: 1}.Build()
	drainingMasterWithDataTemplateState := func(otherMasterReady bool) *downscaleState {
		pod := func(statefulSet appsv1.StatefulSet, ordinal int32, ready bool) corev1.Pod {
			return sset.TestPod{
				Namespace: statefulSet.Namespace, Name: sset.PodName(statefulSet.Name, ordinal),
				StatefulSetName: statefulSet.Name, ClusterName: clusterName, Master: true, Data: true, Ready: ready,
			}.Build()
		}
		return newDownscaleState(
			[]corev1.Pod{pod(ssetChangedToData, 0, true), pod(ssetChangedToData, 1, false), pod(ssetOtherMaster, 0, otherMasterReady)},
			es_sset.StatefulSetList{ssetChangedToData, ssetOtherMaster},
			esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Name: "changed", Count: 1}, {Name: "other", Count: 2}}}},
			set.Make("changed-1"),
			nil,
		)
	}

	// the next master to remove of the master StatefulSet b is draining, while a also needs to remove a master
	ssetMastersA := sset.TestSset{Namespace: ssetMaster3Replicas.Namespace, Name: "a", ClusterName: clusterName, Master: true, Replicas: 2}.Build()
	ssetMastersB := sset.TestSset{Namespace: ssetMaster3Replicas.Namespace, Name: "b", ClusterName: clusterName, Master: true, Replicas: 2}.Build()
	mastersPod := func(statefulSet appsv1.StatefulSet, ordinal int32, ready bool) corev1.Pod {
		return sset.TestPod{
			Namespace: statefulSet.Namespace, Name: sset.PodName(statefulSet.Name, ordinal),
			StatefulSetName: statefulSet.Name, ClusterName: clusterName, Master: true, Ready: ready,
		}.Build()
	}
	drainingMasterInAnotherStatefulSetState := func() *downscaleState {
		return newDownscaleState(
			[]corev1.Pod{mastersPod(ssetMastersA, 0, true), mastersPod(ssetMastersA, 1, true), mastersPod(ssetMastersB, 0, true), mastersPod(ssetMastersB, 1, false)},
			es_sset.StatefulSetList{ssetMastersA, ssetMastersB},
			esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Name: "a", Count: 1}, {Name: "b", Count: 1}}}},
			set.Make("b-1"),
			nil,
		)
	}

	tests := []struct {
		name             string
		state            *downscaleState
		statefulSet      appsv1.StatefulSet
		requestedDeletes int32 // 1 if not set
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
			name:             "should not allow removing a terminating draining data node if maxUnavailable disallows",
			state:            terminatingDrainingDataState,
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name: "should not allow removing a terminating draining master if no other master is running",
			state: &downscaleState{
				runningMasters: 0, masterRemovalInProgress: false, removalsAllowed: new(int32(1)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)), drainingMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name: "should not allow removing a terminating draining master if maxUnavailable disallows",
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)), drainingMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name: "should allow removing a draining data node if maxUnavailable disallows",
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)),
				unavailableDrainingPods: set.Make(sset.PodName(ssetData4Replicas.Name, 3)),
			},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name: "should allow removing a draining master if another master is running",
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)), unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
				unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name: "should allow removing a draining master if maxUnavailable disallows and the other masters are available",
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0)), unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
				unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name: "should not allow removing a draining master if maxUnavailable disallows and another master is unavailable",
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)), unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2), "other-master-0"),
				unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name:             "should not allow removing a draining master if maxUnavailable disallows and another master is missing",
			state:            drainingMasterWithMissingMasterState,
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name:             "should allow removing a draining master of a StatefulSet with a data-only template if the other masters are available",
			state:            drainingMasterWithDataTemplateState(true),
			statefulSet:      ssetChangedToData,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing a draining master of a StatefulSet with a data-only template if another master is unavailable",
			state:            drainingMasterWithDataTemplateState(false),
			statefulSet:      ssetChangedToData,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name: "should not allow removing a draining master if no other master is running",
			state: &downscaleState{
				runningMasters: 0, masterRemovalInProgress: false, removalsAllowed: new(int32(0)), unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
				unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name:             "should not allow removing a draining master if the only other ready master is terminating",
			state:            drainingMasterWithTerminatingMasterState,
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name: "should not allow removing a master if another master is draining",
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1)),
				drainingMasters: set.Make("other-master-1", "other-master-0"),
			},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant + ": other-master-0, other-master-1 draining",
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
			name:             "should not allow removing a master if the next master to remove of another StatefulSet is draining",
			state:            drainingMasterInAnotherStatefulSetState(),
			statefulSet:      ssetMastersA,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant + ": b-1 draining",
		},
		{
			name:             "should allow removing the draining master of a StatefulSet while another StatefulSet needs to remove a master",
			state:            drainingMasterInAnotherStatefulSetState(),
			statefulSet:      ssetMastersB,
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
		{
			name:             "should report why the other masters are held when removing one master",
			state:            &downscaleState{runningMasters: 3, masterRemovalInProgress: false},
			statefulSet:      ssetMaster3Replicas,
			requestedDeletes: 2,
			wantCanDownscale: true,
			wantReason:       OneMasterAtATimeInvariant,
		},
		{
			name:             "should report why the other data nodes are held when maxUnavailable allows some removals",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(1))},
			statefulSet:      ssetData4Replicas,
			requestedDeletes: 3,
			wantCanDownscale: true,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name:             "should not allow removing a data node that is not a member of the cluster",
			state:            &downscaleState{runningMasters: 1, removalsAllowed: new(int32(1)), notMembers: set.Make(sset.PodName(ssetData4Replicas.Name, 3))},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: false,
			wantReason:       NodeNotInClusterInvariant + ": " + sset.PodName(ssetData4Replicas.Name, 3),
		},
		{
			name:             "should report the node that is not a member of the cluster when it holds the removal of the other data nodes",
			state:            &downscaleState{runningMasters: 1, removalsAllowed: new(int32(3)), notMembers: set.Make(sset.PodName(ssetData4Replicas.Name, 2))},
			statefulSet:      ssetData4Replicas,
			requestedDeletes: 3,
			wantCanDownscale: true,
			wantReason:       NodeNotInClusterInvariant + ": " + sset.PodName(ssetData4Replicas.Name, 2),
		},
		{
			name:             "should not hold the removal of the data nodes with a higher ordinal than a node that is not a member of the cluster",
			state:            &downscaleState{runningMasters: 1, removalsAllowed: new(int32(1)), notMembers: set.Make(sset.PodName(ssetData4Replicas.Name, 2))},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing a master that is not a member of the cluster",
			state:            &downscaleState{runningMasters: 2, removalsAllowed: new(int32(1)), notMembers: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2))},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       NodeNotInClusterInvariant + ": " + sset.PodName(ssetMaster3Replicas.Name, 2),
		},
		{
			name:             "should not report a reason when all the data nodes can be removed",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(3))},
			statefulSet:      ssetData4Replicas,
			requestedDeletes: 3,
			wantCanDownscale: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestedDeletes := tt.requestedDeletes
			if requestedDeletes == 0 {
				requestedDeletes = 1
			}
			removals, reason := checkDownscaleInvariants(*tt.state, tt.statefulSet, requestedDeletes)
			canDownscale := removals.count > 0
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
		removals    nodeRemovals
		state       *downscaleState
		wantState   *downscaleState
	}{
		{
			name:        "removing a data node should decrease nodes available for removal",
			statefulSet: ssetData4Replicas,
			removals:    nodeRemovals{count: 1, cost: 1},
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(1))},
			wantState:   &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0))},
		},
		{
			name:        "removing many data nodes should decrease nodes available for removal",
			statefulSet: ssetData4Replicas,
			removals:    nodeRemovals{count: 3, cost: 3},
			state:       &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(3))},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0))},
		},
		{
			name:        "removing a master node should mutate the budget",
			statefulSet: ssetMaster3Replicas,
			removals:    nodeRemovals{count: 1, cost: 1},
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(2))},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: new(int32(1))},
		},
		{
			name:        "removing a draining master node while another master is unavailable should mutate the budget but not the running masters",
			statefulSet: ssetMaster3Replicas,
			removals:    nodeRemovals{count: 1, cost: 1},
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(1)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2), "other-master-0"), unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			wantState: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2), "other-master-0"), unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
		},
		{
			name:        "removing a draining master node that is the only unavailable master should not mutate the budget",
			statefulSet: ssetMaster3Replicas,
			removals:    nodeRemovals{count: 1, drainingRemovals: 1},
			state: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(1)),
				unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)), unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
			wantState: &downscaleState{
				runningMasters: 2, masterRemovalInProgress: true, removalsAllowed: new(int32(0)), drainingRemovalsAllowed: new(int32(0)),
				unavailableMasters: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)), unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			},
		},
		{
			name:        "removing draining and ready data nodes should only count the ready ones in the budget",
			statefulSet: ssetData4Replicas,
			removals:    nodeRemovals{count: 2, cost: 1, drainingRemovals: 1},
			state: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(1)),
				unavailableDrainingPods: set.Make(sset.PodName(ssetData4Replicas.Name, 3)), drainingRemovalsAllowed: new(int32(1)),
			},
			wantState: &downscaleState{
				runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: new(int32(0)),
				unavailableDrainingPods: set.Make(sset.PodName(ssetData4Replicas.Name, 3)), drainingRemovalsAllowed: new(int32(0)),
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
		unavailableDrainingPods set.StringSet
		noMoreThan              int32
		want                    nodeRemovals
	}{
		{
			name:       "no budget",
			noMoreThan: 3,
			want:       nodeRemovals{count: 3, cost: 3},
		},
		{
			name:            "budget limits the removals",
			removalsAllowed: new(int32(2)),
			noMoreThan:      3,
			want:            nodeRemovals{count: 2, cost: 2},
		},
		{
			name:                    "draining nodes do not consume the budget",
			removalsAllowed:         new(int32(0)),
			unavailableDrainingPods: set.Make(podName(3), podName(2)),
			noMoreThan:              3,
			want:                    nodeRemovals{count: 2, drainingRemovals: 2},
		},
		{
			name:                    "draining nodes and ready nodes share the removals",
			removalsAllowed:         new(int32(1)),
			unavailableDrainingPods: set.Make(podName(2)),
			noMoreThan:              3,
			want:                    nodeRemovals{count: 2, cost: 1, drainingRemovals: 1},
		},
		{
			name:                    "a draining node behind a ready node needs budget for the ready node",
			removalsAllowed:         new(int32(0)),
			unavailableDrainingPods: set.Make(podName(2)),
			noMoreThan:              3,
			want:                    nodeRemovals{},
		},
		{
			name:                    "draining removals are limited",
			removalsAllowed:         new(int32(0)),
			drainingRemovalsAllowed: new(int32(1)),
			unavailableDrainingPods: set.Make(podName(3), podName(2)),
			noMoreThan:              3,
			want:                    nodeRemovals{count: 1, drainingRemovals: 1},
		},
		{
			name:                    "draining removals and budget are limited independently",
			removalsAllowed:         new(int32(1)),
			drainingRemovalsAllowed: new(int32(1)),
			unavailableDrainingPods: set.Make(podName(3), podName(1)),
			noMoreThan:              3,
			want:                    nodeRemovals{count: 2, cost: 1, drainingRemovals: 1},
		},
		{
			name:                    "draining nodes of another StatefulSet do not allow removals",
			removalsAllowed:         new(int32(0)),
			unavailableDrainingPods: set.Make(sset.PodName(ssetMaster3Replicas.Name, 2)),
			noMoreThan:              3,
			want:                    nodeRemovals{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := downscaleState{removalsAllowed: tt.removalsAllowed, drainingRemovalsAllowed: tt.drainingRemovalsAllowed, unavailableDrainingPods: tt.unavailableDrainingPods}
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
