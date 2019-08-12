// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func TestNewDownscaleInvariants(t *testing.T) {
	es := v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: ssetMaster3Replicas.Namespace, Name: "name"}}
	tests := []struct {
		name             string
		initialResources []runtime.Object
		want             *DownscaleInvariants
	}{
		{
			name:             "no resources in the apiserver",
			initialResources: nil,
			want:             &DownscaleInvariants{masterRemoved: false, runningMasters: 0},
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
			want: &DownscaleInvariants{masterRemoved: false, runningMasters: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrapClient(fake.NewFakeClient(tt.initialResources...))
			got, err := NewDownscaleInvariants(k8sClient, es)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewDownscaleInvariants() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_DownscaleInvariants_canDownscale(t *testing.T) {
	tests := []struct {
		name             string
		budget           *DownscaleInvariants
		statefulSet      appsv1.StatefulSet
		wantCanDownscale bool
		wantReason       string
	}{
		{
			name:             "should always allow removing data nodes",
			budget:           &DownscaleInvariants{runningMasters: 1, masterRemoved: true},
			statefulSet:      ssetData4Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should allow removing one master if there is another one running",
			budget:           &DownscaleInvariants{runningMasters: 2, masterRemoved: false},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: true,
		},
		{
			name:             "should not allow removing the last master",
			budget:           &DownscaleInvariants{runningMasters: 1, masterRemoved: false},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       AtLeastOneRunningMasterInvariant,
		},
		{
			name:             "should not allow removing a master if one is already being removed",
			budget:           &DownscaleInvariants{runningMasters: 2, masterRemoved: true},
			statefulSet:      ssetMaster3Replicas,
			wantCanDownscale: false,
			wantReason:       OneMasterAtATimeInvariant,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canDownscale, reason := tt.budget.canDownscale(tt.statefulSet)
			if canDownscale != tt.wantCanDownscale {
				t.Errorf("canDownscale() canDownscale = %v, want %v", canDownscale, tt.wantCanDownscale)
			}
			if reason != tt.wantReason {
				t.Errorf("canDownscale() reason = %v, want %v", reason, tt.wantReason)
			}
		})
	}
}

func Test_DownscaleInvariants_accountOneRemoval(t *testing.T) {
	tests := []struct {
		name           string
		statefulSet    appsv1.StatefulSet
		budget         *DownscaleInvariants
		wantInvariants *DownscaleInvariants
	}{
		{
			name:           "removing a data node should be a no-op",
			statefulSet:    ssetData4Replicas,
			budget:         &DownscaleInvariants{runningMasters: 2, masterRemoved: false},
			wantInvariants: &DownscaleInvariants{runningMasters: 2, masterRemoved: false},
		},
		{
			name:           "removing a master node should mutate the budget",
			statefulSet:    ssetMaster3Replicas,
			budget:         &DownscaleInvariants{runningMasters: 2, masterRemoved: false},
			wantInvariants: &DownscaleInvariants{runningMasters: 1, masterRemoved: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.budget.accountOneRemoval(tt.statefulSet)
			require.Equal(t, tt.wantInvariants, tt.budget)
		})
	}
}
