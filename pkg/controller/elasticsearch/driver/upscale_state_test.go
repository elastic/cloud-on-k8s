// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"math"
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
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
		name        string
		state       *upscaleState
		actual      appsv1.StatefulSet
		ssetToApply appsv1.StatefulSet
		wantSset    appsv1.StatefulSet
		wantState   *upscaleState
	}{
		{
			name:        "no change on the sset spec",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actual:      sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
		{
			name:        "spec change (same replicas)",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true},
			actual:      sset.TestSset{Name: "sset", Version: "6.8.0", Replicas: 3, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Version: "7.2.0", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Version: "7.2.0", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true},
		},
		{
			name:        "upscale data nodes from 1 to 3: should go through",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 2},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 2, recordedCreates: 2},
		},
		{
			name:        "upscale data nodes from 1 to 4: should limit to 3",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 2},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 4, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 2, recordedCreates: 2},
		},
		{
			name:        "upscale master nodes from 1 to 3: should limit to 2",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 1},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 2, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: 1, recordedCreates: 1},
		},
		{
			name:        "upscale master nodes from 1 to 3 when cluster not yet bootstrapped: should go through",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: 2},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: 2, recordedCreates: 2},
		},
		{
			name:        "new StatefulSet with 5 master nodes, cluster isn't bootstrapped yet: should go through",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: 3},
			actual:      appsv1.StatefulSet{},
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: 3, recordedCreates: 3},
		},
		{
			name:        "new StatefulSet with 5 master nodes, cluster already bootstrapped: should limit to 1",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 1},
			actual:      appsv1.StatefulSet{},
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: 1, recordedCreates: 1},
		},
		{
			name:  "scale up from 3 to 5, nodespec changed to master: should limit to 4 (one new master)",
			state: &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: 1},
			// no master on existing StatefulSet
			actual: sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			// turned into masters on newer StatefulSet
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 5, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 4, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: 1, recordedCreates: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSset := tt.state.limitNodesCreation(tt.actual, tt.ssetToApply)
			// StatefulSet should be adapted
			require.Equal(t, gotSset, tt.wantSset)
			// upscaleState should be mutated accordingly
			require.Equal(t, tt.state, tt.wantState)
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
		ctx      upscaleCtx
		actual   sset.StatefulSetList
		expected nodespec.ResourcesList
	}
	tests := []struct {
		name string
		args args
		want *upscaleState
	}{
		{
			name: "cluster not bootstrapped",
			args: args{ctx: upscaleCtx{es: *notBootstrappedES()}},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: math.MaxInt32},
		},
		{
			name: "bootstrapped, no master node joining",
			args: args{ctx: upscaleCtx{k8sClient: k8s.WrapClient(fake.NewFakeClient()), es: *bootstrappedES()}},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: math.MaxInt32},
		},
		{
			name: "bootstrapped, a master node is pending",
			args: args{
				ctx: upscaleCtx{
					k8sClient: k8s.WrapClient(fake.NewFakeClient(sset.TestPod{ClusterName: "cluster", Master: true, Status: corev1.PodStatus{Phase: corev1.PodPending}}.BuildPtr())),
					es:        *bootstrappedES(),
				},
			},
			want: &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: math.MaxInt32, recordedCreates: 1},
		},
		{
			name: "bootstrapped, a data node is pending",
			args: args{
				ctx: upscaleCtx{
					k8sClient: k8s.WrapClient(fake.NewFakeClient(sset.TestPod{ClusterName: "cluster", Master: false, Data: true, Status: corev1.PodStatus{Phase: corev1.PodPending}}.BuildPtr())),
					es:        *bootstrappedES(),
				},
			},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: math.MaxInt32},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newUpscaleState(tt.args.ctx, tt.args.actual, tt.args.expected)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newUpscaleState() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newUpscaleStateWithChangeBudget(t *testing.T) {
	type test struct {
		name     string
		ctx      upscaleCtx
		actual   sset.StatefulSetList
		expected nodespec.ResourcesList
		want     *upscaleState
	}

	type args struct {
		name           string
		actual         []int
		expected       []int
		maxSurge       int
		createsAllowed int
	}

	getTest := func(args args) test {
		var actualSsets sset.StatefulSetList
		for _, count := range args.actual {
			actualSsets = append(actualSsets, sset.TestSset{Name: "sset", Replicas: int32(count), Master: false}.Build())
		}

		var expectedResources nodespec.ResourcesList
		for _, count := range args.expected {
			expectedResources = append(expectedResources, nodespec.Resources{StatefulSet: sset.TestSset{Name: "sset", Replicas: int32(count), Master: false}.Build()})
		}

		return test{
			name: args.name,
			ctx: upscaleCtx{
				k8sClient: k8s.WrapClient(fake.NewFakeClient()),
				es:        *bootstrappedESWithChangeBudget(args.maxSurge, 0),
			},
			actual:   actualSsets,
			expected: expectedResources,
			want:     &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: int32(args.createsAllowed)},
		}
	}
	defaultTest := getTest(args{name: "5 nodes present, 5 nodes target, n/a maxSurge - maxint32 creates allowed", actual: []int{3}, expected: []int{3}, maxSurge: 0, createsAllowed: math.MaxInt32})
	defaultTest.ctx.es.Spec.UpdateStrategy = v1alpha1.UpdateStrategy{}

	tests := []test{
		getTest(args{name: "3 nodes present, 3 nodes target - no creates allowed", actual: []int{3}, expected: []int{3}, maxSurge: 0, createsAllowed: 0}),
		getTest(args{name: "2 ssets, 6 nodes present, 6 nodes target - no creates allowed", actual: []int{3, 3}, expected: []int{3, 3}, maxSurge: 0, createsAllowed: 0}),
		getTest(args{name: "2 nodes present, 3 nodes target - 1 create allowed", actual: []int{2}, expected: []int{3}, maxSurge: 0, createsAllowed: 1}),
		getTest(args{name: "0 nodes present, 5 nodes target, 3 maxSurge - 8 creates allowed", actual: []int{}, expected: []int{5}, maxSurge: 3, createsAllowed: 8}),
		getTest(args{name: "5 nodes present, 3 nodes target, 3 maxSurge - 1 create allowed", actual: []int{5}, expected: []int{3}, maxSurge: 3, createsAllowed: 1}),
		defaultTest,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newUpscaleState(tt.ctx, tt.actual, tt.expected)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newUpscaleState() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_calculateCreatesAllowed(t *testing.T) {
	type args struct {
		name         string
		changeBudget *v1alpha1.ChangeBudget
		actual       int32
		expected     int32
		want         int32
	}

	tests := []args{
		{name: "nil budget, 5->5, want max", changeBudget: nil, actual: 5, expected: 5, want: math.MaxInt32},
		{name: "max budget, 1->2, want max", changeBudget: &v1alpha1.ChangeBudget{MaxSurge: math.MaxInt32}, actual: 1, expected: 3, want: math.MaxInt32},
		{name: "max-1 budget, 1->2, want max", changeBudget: &v1alpha1.ChangeBudget{MaxSurge: math.MaxInt32 - 1}, actual: 1, expected: 3, want: math.MaxInt32},
		{name: "0 budget, 5->5, want 0", changeBudget: &v1alpha1.ChangeBudget{MaxSurge: 0}, actual: 5, expected: 5, want: 0},
		{name: "1 budget, 5->5, want 1", changeBudget: &v1alpha1.ChangeBudget{MaxSurge: 1}, actual: 5, expected: 5, want: 1},
		{name: "2 budget, 5->5, want 2", changeBudget: &v1alpha1.ChangeBudget{MaxSurge: 2}, actual: 5, expected: 5, want: 2},
		{name: "2 budget, 3->5, want 4", changeBudget: &v1alpha1.ChangeBudget{MaxSurge: 2}, actual: 3, expected: 5, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCreatesAllowed(tt.changeBudget, tt.actual, tt.expected)
			if got != tt.want {
				t.Errorf("calculateCreatesAllowed() got = %d, want %d", got, tt.want)
			}
		})
	}
}
