// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_upscaleState_limitMasterNodesCreation(t *testing.T) {
	tests := []struct {
		name               string
		state              *upscaleState
		actualStatefulSets sset.StatefulSetList
		ssetToApply        appsv1.StatefulSet
		wantSset           appsv1.StatefulSet
		wantState          *upscaleState
	}{
		{
			name:               "no change on the sset spec",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build()},
			ssetToApply:        sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:          &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
		{
			name:               "spec change (same replicas)",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Version: "6.8.0", Replicas: 3, Master: true}.Build()},
			ssetToApply:        sset.TestSset{Name: "sset", Version: "7.2.0", Replicas: 3, Master: true}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Version: "7.2.0", Replicas: 3, Master: true}.Build(),
			wantState:          &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
		{
			name:               "upscale data nodes from 1 to 3: should go through",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build()},
			ssetToApply:        sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:          &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
		{
			name:               "upscale master nodes from 1 to 3: should limit to 2",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build()},
			ssetToApply:        sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Replicas: 2, Master: true}.Build(),
			wantState:          &upscaleState{allowMasterCreation: false, isBootstrapped: true},
		},
		{
			name:               "upscale master nodes from 1 to 3 when cluster not yet bootstrapped: should go through",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: false},
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build()},
			ssetToApply:        sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:          &upscaleState{allowMasterCreation: true, isBootstrapped: false},
		},
		{
			name:               "new StatefulSet with 5 master nodes, cluster isn't bootstrapped yet: should go through",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: false},
			actualStatefulSets: sset.StatefulSetList{},
			ssetToApply:        sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:          &upscaleState{allowMasterCreation: true, isBootstrapped: false},
		},
		{
			name:               "new StatefulSet with 5 master nodes, cluster already bootstrapped: should limit to 1",
			state:              &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actualStatefulSets: sset.StatefulSetList{},
			ssetToApply:        sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:           sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			wantState:          &upscaleState{allowMasterCreation: false, isBootstrapped: true},
		},
		{
			name:  "scale up from 3 to 5, nodespec changed to master: should limit to 4 (one new master)",
			state: &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			// no master on existing StatefulSet
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build()},
			// turned into masters on newer StatefulSet
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 5, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 4, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: false, isBootstrapped: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSset := tt.state.limitMasterNodesCreation(tt.actualStatefulSets, tt.ssetToApply)
			// StatefulSet should be adapted
			require.Equal(t, gotSset, tt.wantSset)
			// upscaleState should be mutated accordingly
			require.Equal(t, tt.wantState, tt.state)
		})
	}
}

type fakeESState struct {
	ESState
}

func (f *fakeESState) NodesInCluster(nodeNames []string) (bool, error) {
	if nodeNames[0] == "inCluster" {
		return true, nil
	}
	return false, nil
}

func Test_isMasterNodeJoining(t *testing.T) {
	tests := []struct {
		name    string
		pod     v1.Pod
		esState ESState
		want    bool
	}{
		{
			name: "pod pending",
			pod:  v1.Pod{Status: v1.PodStatus{Phase: v1.PodPending}},
			want: true,
		},
		{
			name: "pod running but not ready",
			pod: v1.Pod{Status: v1.PodStatus{
				Phase: v1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				}}},
			want: true,
		},
		{
			name: "pod running and ready but not in the cluster yet",
			pod: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "notInCluster",
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					}}},
			esState: &fakeESState{},
			want:    true,
		},
		{
			name: "pod running and ready and in the cluster",
			pod: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "inCluster",
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					}}},
			esState: &fakeESState{},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isMasterNodeJoining(tt.pod, tt.esState)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("isMasterNodeJoining() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newUpscaleState(t *testing.T) {
	type args struct {
		c       k8s.Client
		es      v1alpha1.Elasticsearch
		esState ESState
	}
	tests := []struct {
		name string
		args args
		want *upscaleState
	}{
		{
			name: "cluster not bootstrapped",
			args: args{
				es: notBootstrappedES,
			},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: false},
		},
		{
			name: "bootstrapped, no master node joining",
			args: args{
				c:  k8s.WrapClient(fake.NewFakeClient()),
				es: bootstrappedES,
			},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
		{
			name: "bootstrapped, a master node is pending",
			args: args{
				c:  k8s.WrapClient(fake.NewFakeClient(sset.TestPod{ClusterName: "cluster", Master: true, Status: corev1.PodStatus{Phase: corev1.PodPending}}.BuildPtr())),
				es: bootstrappedES,
			},
			want: &upscaleState{allowMasterCreation: false, isBootstrapped: true},
		},
		{
			name: "bootstrapped, a data node is pending",
			args: args{
				c:  k8s.WrapClient(fake.NewFakeClient(sset.TestPod{ClusterName: "cluster", Master: false, Data: true, Status: corev1.PodStatus{Phase: corev1.PodPending}}.BuildPtr())),
				es: bootstrappedES,
			},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newUpscaleState(tt.args.c, tt.args.es, tt.args.esState)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newUpscaleState() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_upscaleStateBuilder_InitOnce(t *testing.T) {
	b := &upscaleStateBuilder{}
	s, err := b.InitOnce(k8s.WrapClient(fake.NewFakeClient()), notBootstrappedES, &fakeESState{})
	require.NoError(t, err)
	require.Equal(t, &upscaleState{isBootstrapped: false, allowMasterCreation: true}, s)
	// running InitOnce again should not build the state again
	// run it with arguments that should normally modify the state
	s, err = b.InitOnce(k8s.WrapClient(fake.NewFakeClient()), bootstrappedES, &fakeESState{})
	require.NoError(t, err)
	require.Equal(t, &upscaleState{isBootstrapped: false, allowMasterCreation: true}, s)
	// double checking this would indeed modify the state on first init
	b = &upscaleStateBuilder{}
	s, err = b.InitOnce(k8s.WrapClient(fake.NewFakeClient()), bootstrappedES, &fakeESState{})
	require.NoError(t, err)
	require.Equal(t, &upscaleState{isBootstrapped: true, allowMasterCreation: true}, s)

}
