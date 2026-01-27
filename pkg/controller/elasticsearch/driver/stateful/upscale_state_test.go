// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
			state:       &upscaleState{},
			actual:      sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{},
		},
		{
			name:        "spec change (same replicas)",
			state:       &upscaleState{},
			actual:      sset.TestSset{Name: "sset", Version: "7.17.0", Replicas: 3, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Version: "8.0.0", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Version: "8.0.0", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{},
		},
		{
			name:        "upscale nodes from 1 to 3: should go through",
			state:       &upscaleState{createsAllowed: ptr.To[int32](2)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{createsAllowed: ptr.To[int32](2), recordedCreates: 2},
		},
		{
			name:        "upscale nodes from 1 to 4: should limit to 3",
			state:       &upscaleState{createsAllowed: ptr.To[int32](2)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: false}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 4, Master: false}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: false}.Build(),
			wantState:   &upscaleState{createsAllowed: ptr.To[int32](2), recordedCreates: 2},
		},
		{
			name:        "upscale nodes from 1 to 3 (one allowed by maxSurge): should limit to 2",
			state:       &upscaleState{createsAllowed: ptr.To[int32](1)},
			actual:      sset.TestSset{Name: "sset", Replicas: 1, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 2, Master: true}.Build(),
			wantState:   &upscaleState{createsAllowed: ptr.To[int32](1), recordedCreates: 1},
		},
		{
			name:        "upscale nodes from 3 to 4, but no creates allowed: should limit to 0",
			state:       &upscaleState{createsAllowed: ptr.To[int32](0)},
			actual:      sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			ssetToApply: sset.TestSset{Name: "sset", Replicas: 4, Master: true}.Build(),
			wantSset:    sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build(),
			wantState:   &upscaleState{createsAllowed: ptr.To[int32](0), recordedCreates: 0},
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

func Test_newUpscaleState(t *testing.T) {
	esWithMaxSurge := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster",
			Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "uuid"},
		},
		Spec: esv1.ElasticsearchSpec{Version: "7.3.0", UpdateStrategy: esv1.UpdateStrategy{ChangeBudget: esv1.ChangeBudget{MaxSurge: ptr.To[int32](3)}}},
	}

	esWithoutMaxSurge := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       esv1.ElasticsearchSpec{Version: "7.3.0"},
	}
	type args struct {
		ctx      upscaleCtx
		actual   es_sset.StatefulSetList
		expected nodespec.ResourcesList
	}
	tests := []struct {
		name string
		args args
		want *upscaleState
	}{
		{
			name: "no stateful sets",
			args: args{ctx: upscaleCtx{es: esWithoutMaxSurge}},
			want: &upscaleState{createsAllowed: nil},
		},
		{
			name: "expected stateful sets, no maxSurge",
			args: args{
				ctx:      upscaleCtx{k8sClient: k8s.NewFakeClient(), es: esWithoutMaxSurge},
				expected: nodespec.ResourcesList{nodespec.Resources{StatefulSet: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build()}},
			},
			want: &upscaleState{createsAllowed: nil},
		},
		{
			name: "expected stateful sets, maxSurge",
			args: args{
				ctx:      upscaleCtx{k8sClient: k8s.NewFakeClient(), es: esWithMaxSurge},
				expected: nodespec.ResourcesList{nodespec.Resources{StatefulSet: sset.TestSset{Name: "sset", Replicas: 3, Master: true}.Build()}},
			},
			want: &upscaleState{createsAllowed: ptr.To[int32](6)}, // 3 maxSurge + 3 expected - 0 actual = 6 creates allowed
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newUpscaleState(tt.args.ctx, tt.args.actual, tt.args.expected)
			got.ctx = upscaleCtx{}
			require.Equal(t, tt.want, got)
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
		actual   es_sset.StatefulSetList
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
		var actualSsets es_sset.StatefulSetList
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
				k8sClient: k8s.NewFakeClient(),
				es:        bootstrappedESWithChangeBudget(args.maxSurge, ptr.To[int32](0)),
			},
			actual:   actualSsets,
			expected: expectedResources,
			want:     &upscaleState{createsAllowed: args.createsAllowed},
		}
	}
	defaultTest := getTest(args{actual: []int{3}, expected: []int{3}, maxSurge: nil, createsAllowed: nil, name: "5 nodes present, 5 nodes target, n/a maxSurge - unbounded creates allowed"})

	tests := []test{
		getTest(args{actual: []int{3}, expected: []int{3}, maxSurge: ptr.To[int32](0), createsAllowed: ptr.To[int32](0), name: "3 nodes present, 3 nodes target - no creates allowed"}),
		getTest(args{actual: []int{3, 3}, expected: []int{3, 3}, maxSurge: ptr.To[int32](0), createsAllowed: ptr.To[int32](0), name: "2 ssets, 6 nodes present, 6 nodes target - no creates allowed"}),
		getTest(args{actual: []int{2}, expected: []int{3}, maxSurge: ptr.To[int32](0), createsAllowed: ptr.To[int32](1), name: "2 nodes present, 3 nodes target - 1 create allowed"}),
		getTest(args{actual: []int{}, expected: []int{5}, maxSurge: ptr.To[int32](3), createsAllowed: ptr.To[int32](8), name: "0 nodes present, 5 nodes target, 3 maxSurge - 8 creates allowed"}),
		getTest(args{actual: []int{5}, expected: []int{3}, maxSurge: ptr.To[int32](3), createsAllowed: ptr.To[int32](1), name: "5 nodes present, 3 nodes target, 3 maxSurge - 1 create allowed"}),
		defaultTest,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newUpscaleState(tt.ctx, tt.actual, tt.expected)
			got.ctx = upscaleCtx{}
			require.Equal(t, tt.want, got)
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
		{name: "0 budget, 5->5, want 0", maxSurge: ptr.To[int32](0), actual: 5, expected: 5, want: ptr.To[int32](0)},
		{name: "1 budget, 5->5, want 1", maxSurge: ptr.To[int32](1), actual: 5, expected: 5, want: ptr.To[int32](1)},
		{name: "2 budget, 5->5, want 2", maxSurge: ptr.To[int32](2), actual: 5, expected: 5, want: ptr.To[int32](2)},
		{name: "2 budget, 3->5, want 4", maxSurge: ptr.To[int32](2), actual: 3, expected: 5, want: ptr.To[int32](4)},
		{name: "6 budget, 10->5, want 4", maxSurge: ptr.To[int32](6), actual: 10, expected: 5, want: ptr.To[int32](1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCreatesAllowed(tt.maxSurge, tt.actual, tt.expected)
			require.Equal(t, tt.want, got)
		})
	}
}
