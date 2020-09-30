// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_upscaleState_limitNodesCreation(t *testing.T) {
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
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(2)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(2), recordedCreates: 2},
		},
		{
			name:        "upscale data nodes from 1 to 4: should limit to 3",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(2)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 4, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(2), recordedCreates: 2},
		},
		{
			name:        "upscale master nodes from 1 to 3: should limit to 2",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(1)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 2, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: pointer.Int32(1), recordedCreates: 1},
		},
		{
			name:        "upscale master nodes from 1 to 3 when cluster not yet bootstrapped: should go through",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: pointer.Int32(2)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: pointer.Int32(2), recordedCreates: 2},
		},
		{
			name:        "upscale masters from 3 to 4, but no creates allowed: should limit to 0",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(0)},
			actual:      sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 4, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(0), recordedCreates: 0},
		},
		{
			name:        "upscale data nodes from 3 to 4, but no creates allowed: should limit to 0",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(0)},
			actual:      sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 4, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(0), recordedCreates: 0},
		},
		{
			name:        "new StatefulSet with 5 master nodes, cluster isn't bootstrapped yet: should go through",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: pointer.Int32(3)},
			actual:      appsv1.StatefulSet{},
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: pointer.Int32(3), recordedCreates: 3},
		},
		{
			name:        "new StatefulSet with 5 master nodes, cluster already bootstrapped: should limit to 1",
			state:       &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: pointer.Int32(1)},
			actual:      appsv1.StatefulSet{},
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			wantState:   &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: pointer.Int32(1), recordedCreates: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSset, err := tt.state.limitNodesCreation(tt.actual, tt.ssetToApply)
			require.NoError(t, err)
			// StatefulSet should be adapted
			require.Equal(t, tt.wantSset, gotSset)
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
		pod     corev1.Pod
		esState ESState
		want    bool
	}{
		{
			name: "pod pending",
			pod:  corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}},
			want: true,
		},
		{
			name: "pod running but not ready",
			pod: corev1.Pod{Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
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
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "notInCluster",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
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
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "inCluster",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
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
	bootstrappedES := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster",
			Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "uuid"},
		},
		Spec: esv1.ElasticsearchSpec{Version: "7.3.0"},
	}

	notBootstrappedES := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       esv1.ElasticsearchSpec{Version: "7.3.0"},
	}
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
			args: args{ctx: upscaleCtx{es: notBootstrappedES}},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: false, createsAllowed: nil},
		},
		{
			name: "bootstrapped, no master node joining",
			args: args{ctx: upscaleCtx{k8sClient: k8s.WrappedFakeClient(), es: bootstrappedES}},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: nil},
		},
		{
			name: "bootstrapped, a master node is pending",
			args: args{
				ctx: upscaleCtx{
					k8sClient: k8s.WrappedFakeClient(sset.TestPod{ClusterName: "cluster", Master: true, Phase: corev1.PodPending}.BuildPtr()),
					es:        bootstrappedES,
				},
			},
			want: &upscaleState{allowMasterCreation: false, isBootstrapped: true, createsAllowed: nil, recordedCreates: 1},
		},
		{
			name: "bootstrapped, a data node is pending",
			args: args{
				ctx: upscaleCtx{
					k8sClient: k8s.WrappedFakeClient(sset.TestPod{ClusterName: "cluster", Master: false, Data: true, Phase: corev1.PodPending}.BuildPtr()),
					es:        bootstrappedES,
				},
			},
			want: &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newUpscaleState(tt.args.ctx, tt.args.actual, tt.args.expected)
			require.NoError(t, buildOnce(got))
			got.ctx = upscaleCtx{}
			got.once = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newUpscaleState() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func bootstrappedESWithChangeBudget(maxSurge, maxUnavailable *int32) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster",
			Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "uuid"},
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "7.3.0",
			UpdateStrategy: esv1.UpdateStrategy{
				ChangeBudget: esv1.ChangeBudget{
					MaxSurge:       maxSurge,
					MaxUnavailable: maxUnavailable,
				},
			},
		},
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
		maxSurge       *int32
		createsAllowed *int32
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
				k8sClient: k8s.WrappedFakeClient(),
				es:        bootstrappedESWithChangeBudget(args.maxSurge, pointer.Int32(0)),
			},
			actual:   actualSsets,
			expected: expectedResources,
			want:     &upscaleState{allowMasterCreation: true, isBootstrapped: true, createsAllowed: args.createsAllowed},
		}
	}
	defaultTest := getTest(args{actual: []int{3}, expected: []int{3}, maxSurge: nil, createsAllowed: nil, name: "5 nodes present, 5 nodes target, n/a maxSurge - unbounded creates allowed"})
	defaultTest.ctx.es.Spec.UpdateStrategy = esv1.UpdateStrategy{}

	tests := []test{
		getTest(args{actual: []int{3}, expected: []int{3}, maxSurge: pointer.Int32(0), createsAllowed: pointer.Int32(0), name: "3 nodes present, 3 nodes target - no creates allowed"}),
		getTest(args{actual: []int{3, 3}, expected: []int{3, 3}, maxSurge: pointer.Int32(0), createsAllowed: pointer.Int32(0), name: "2 ssets, 6 nodes present, 6 nodes target - no creates allowed"}),
		getTest(args{actual: []int{2}, expected: []int{3}, maxSurge: pointer.Int32(0), createsAllowed: pointer.Int32(1), name: "2 nodes present, 3 nodes target - 1 create allowed"}),
		getTest(args{actual: []int{}, expected: []int{5}, maxSurge: pointer.Int32(3), createsAllowed: pointer.Int32(8), name: "0 nodes present, 5 nodes target, 3 maxSurge - 8 creates allowed"}),
		getTest(args{actual: []int{5}, expected: []int{3}, maxSurge: pointer.Int32(3), createsAllowed: pointer.Int32(1), name: "5 nodes present, 3 nodes target, 3 maxSurge - 1 create allowed"}),
		defaultTest,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newUpscaleState(tt.ctx, tt.actual, tt.expected)
			require.NoError(t, buildOnce(got))
			got.ctx = upscaleCtx{}
			got.once = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newUpscaleState() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_calculateCreatesAllowed(t *testing.T) {
	type args struct {
		name     string
		maxSurge *int32
		actual   int32
		expected int32
		want     *int32
	}
	tests := []args{
		{name: "nil budget, 5->6, want max", maxSurge: nil, actual: 5, expected: 6, want: nil},
		{name: "0 budget, 5->5, want 0", maxSurge: pointer.Int32(0), actual: 5, expected: 5, want: pointer.Int32(0)},
		{name: "1 budget, 5->5, want 1", maxSurge: pointer.Int32(1), actual: 5, expected: 5, want: pointer.Int32(1)},
		{name: "2 budget, 5->5, want 2", maxSurge: pointer.Int32(2), actual: 5, expected: 5, want: pointer.Int32(2)},
		{name: "2 budget, 3->5, want 4", maxSurge: pointer.Int32(2), actual: 3, expected: 5, want: pointer.Int32(4)},
		{name: "6 budget, 10->5, want 4", maxSurge: pointer.Int32(6), actual: 10, expected: 5, want: pointer.Int32(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCreatesAllowed(tt.maxSurge, tt.actual, tt.expected)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculateCreatesAllowed() got = %d, want %d", got, tt.want)
			}
		})
	}
}
