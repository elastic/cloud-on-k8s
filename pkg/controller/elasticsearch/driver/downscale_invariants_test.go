// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"math"
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_newDownscaleState(t *testing.T) {
	es := v1beta1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: ssetMaster3Replicas.Namespace, Name: "name"},
		Spec:       v1beta1.ElasticsearchSpec{Nodes: []v1beta1.NodeSpec{{NodeCount: 4}}},
	}

	tests := []struct {
		name             string
		initialResources []runtime.Object
		want             *downscaleState
	}{
		{
			name:             "no resources in the apiserver",
			initialResources: nil,
			want:             &downscaleState{masterRemovalInProgress: false, runningMasters: 0, removalsAllowed: 0},
		},
		{
			name: "3 masters running in the apiserver, 1 not running",
			initialResources: []runtime.Object{
				// 3 masters running
				&corev1.Pod{
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
				&corev1.Pod{
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
				&corev1.Pod{
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
				&corev1.Pod{
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
			want: &downscaleState{masterRemovalInProgress: false, runningMasters: 3, removalsAllowed: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrapClient(fake.NewFakeClient(tt.initialResources...))
			got, err := newDownscaleState(k8sClient, es)
			require.NoError(t, err)
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
		want           int32
	}{
		{
			name:           "default should be 1",
			nodesReady:     5,
			desiredNodes:   5,
			maxUnavailable: nil,
			want:           1,
		},
		{
			name:           "negative should be effectively unbounded",
			nodesReady:     5,
			desiredNodes:   5,
			maxUnavailable: common.Int32(-1),
			want:           math.MaxInt32,
		},
		{
			name:           "scaling down, at least one node up",
			nodesReady:     10,
			desiredNodes:   3,
			maxUnavailable: common.Int32(2),
			want:           9,
		},
		{
			name:           "scaling up, can't remove anything",
			nodesReady:     3,
			desiredNodes:   5,
			maxUnavailable: common.Int32(1),
			want:           0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRemovalsAllowed(tt.nodesReady, tt.desiredNodes, tt.maxUnavailable)
			if got != tt.want {
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
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: 1},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing data nodes of maxUnavailable disallows",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: 0},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
		{
			name:             "should allow removing one master if there is another one running",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: 1},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing the last master",
			state:            &downscaleState{runningMasters: 1, masterRemovalInProgress: false, removalsAllowed: 1},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name:             "should not allow removing a master if one is already being removed",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: true, removalsAllowed: 2},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant,
		},
		{
			name:             "should not allow removing a master if maxUnavailable disallows",
			state:            &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: 0},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       RespectMaxUnavailableInvariant,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canDownscale, reason := checkDownscaleInvariants(*tt.state, tt.statefulSet)
			if canDownscale != tt.wantCanDownscale {
				t.Errorf("canDownscale() canDownscale = %v, want %v", canDownscale, tt.wantCanDownscale)
			}
			if reason != tt.wantReason {
				t.Errorf("canDownscale() reason = %v, want %v", reason, tt.wantReason)
			}
		})
	}
}

func Test_downscaleState_recordOneRemoval(t *testing.T) {
	tests := []struct {
		name        string
		statefulSet appsv1.StatefulSet
		state       *downscaleState
		wantState   *downscaleState
	}{
		{
			name:        "removing a data node should decrease nodes available for removal",
			statefulSet: ssetData4Replicas,
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: 1},
			wantState:   &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: 0},
		},
		{
			name:        "removing a master node should mutate the budget",
			statefulSet: ssetMaster3Replicas,
			state:       &downscaleState{runningMasters: 2, masterRemovalInProgress: false, removalsAllowed: 2},
			wantState:   &downscaleState{runningMasters: 1, masterRemovalInProgress: true, removalsAllowed: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.state.recordOneRemoval(tt.statefulSet)
			require.Equal(t, tt.wantState, tt.state)
		})
	}
}
