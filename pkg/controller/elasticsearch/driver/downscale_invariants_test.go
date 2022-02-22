// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func Test_newDownscaleState(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: ssetMaster3Replicas.Namespace, Name: "name"},
		Spec:       esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 4}}},
	}

	tests := []struct {
		name       string
		actualPods []corev1.Pod
		want       *downscaleState
	}{
		{
			name:       "no resources in the apiserver",
			actualPods: nil,
			want:       &downscaleState{masterRemovalInProgress: false, runningMasters: 0, removalsAllowed: pointer.Int32(0)},
		},
		{
			name: "3 masters running in the apiserver, 1 not running",
			actualPods: []corev1.Pod{
				// 3 masters running
				corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ssetMaster3Replicas.Namespace,
						Name:      ssetMaster3Replicas.Name + "-0",
						Labels: map[string]string{
							label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
							string(label.NodeTypesMasterLabelName): "true",
							label.ClusterNameLabelName:             es.Name,
						},
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
				corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ssetMaster3Replicas.Namespace,
						Name:      ssetMaster3Replicas.Name + "-1",
						Labels: map[string]string{
							label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
							string(label.NodeTypesMasterLabelName): "true",
							label.ClusterNameLabelName:             es.Name,
						},
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
				corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ssetMaster3Replicas.Namespace,
						Name:      ssetMaster3Replicas.Name + "-2",
						Labels: map[string]string{
							label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
							string(label.NodeTypesMasterLabelName): "true",
							label.ClusterNameLabelName:             es.Name,
						},
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
				corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ssetMaster3Replicas.Namespace,
						Name:      ssetMaster3Replicas.Name + "-3",
						Labels: map[string]string{
							label.StatefulSetNameLabelName:         ssetMaster3Replicas.Name,
							string(label.NodeTypesMasterLabelName): "true",
							label.ClusterNameLabelName:             es.Name,
						},
					},
				},
			},
			want: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: pointer.Int32(0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDownscaleState(tt.actualPods, es)
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
			maxUnavailable: pointer.Int32(2),
			want:           pointer.Int32(9),
		},
		{
			name:           "scaling up, can't remove anything",
			nodesReady:     3,
			desiredNodes:   5,
			maxUnavailable: pointer.Int32(1),
			want:           pointer.Int32(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRemovalsAllowed(tt.nodesReady, tt.desiredNodes, tt.maxUnavailable)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculateRemovalsAllowed() got = %d, want = %d", got, tt.want)
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
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: pointer.Int32(1)},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing data nodes of maxUnavailable disallows",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: pointer.Int32(0)},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name:             "should allow removing one master if there is another one running",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(1)},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing the last master",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(1)},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name:             "should not allow removing a master if one is already being removed",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: true, removalsAllowed: pointer.Int32(2)},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant,
		},
		{
			name:             "should not allow removing a master if maxUnavailable disallows",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(0)},
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
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(1)},
			wantState:   &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(0)},
		},
		{
			name:        "removing many data nodes should decrease nodes available for removal",
			statefulSet: ssetData4Replicas,
			removals:    3,
			state:       &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(3)},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(0)},
		},
		{
			name:        "removing a master node should mutate the budget",
			statefulSet: ssetMaster3Replicas,
			removals:    1,
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: pointer.Int32(2)},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: pointer.Int32(1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.state.recordNodeRemoval(tt.statefulSet, tt.removals)
			require.Equal(t, tt.wantState, tt.state)
		})
	}
}
