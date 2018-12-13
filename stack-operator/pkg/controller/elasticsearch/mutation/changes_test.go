package mutation

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var defaultPod = ESPod(defaultImage, defaultCPULimit)
var defaultPodSpecCtx = ESPodSpecContext(defaultImage, defaultCPULimit)

func TestCalculateChanges(t *testing.T) {
	var taintedPod = defaultPod
	taintedPod.Annotations = map[string]string{TaintedAnnotationName: "true"}
	type args struct {
		expected []support.PodSpecContext
		state    support.ResourcesState
	}
	tests := []struct {
		name string
		args args
		want Changes
	}{
		{
			name: "no changes",
			args: args{
				expected: []support.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "2 new pods",
			args: args{
				expected: []support.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{
				ToKeep: []corev1.Pod{defaultPod, defaultPod},
				ToAdd:  []PodToAdd{{PodSpecCtx: defaultPodSpecCtx}, {PodSpecCtx: defaultPodSpecCtx}},
			},
		},
		{
			name: "2 less pods",
			args: args{
				expected: []support.PodSpecContext{},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{ToRemove: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "1 pod replaced",
			args: args{
				expected: []support.PodSpecContext{defaultPodSpecCtx, ESPodSpecContext("another-image", defaultCPULimit)},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{
				ToKeep:   []corev1.Pod{defaultPod},
				ToRemove: []corev1.Pod{defaultPod},
				ToAdd:    []PodToAdd{{PodSpecCtx: ESPodSpecContext("another-image", defaultCPULimit)}},
			},
		},
		{
			name: "1 pod replaced on pod tainted",
			args: args{
				expected: []support.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{taintedPod, defaultPod}},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod}, ToRemove: []corev1.Pod{defaultPod}, ToAdd: []PodToAdd{PodToAdd{PodSpecCtx: defaultPodSpecCtx}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateChanges(tt.args.expected, tt.args.state, func(ctx support.PodSpecContext) (corev1.Pod, error) {
				return corev1.Pod{}, nil // TODO: fix
			})
			assert.NoError(t, err)
			assert.Equal(t, len(tt.want.ToKeep), len(got.ToKeep))
			assert.Equal(t, len(tt.want.ToAdd), len(got.ToAdd))
			assert.Equal(t, len(tt.want.ToRemove), len(got.ToRemove))
		})
	}
}

func TestChanges_IsEmpty(t *testing.T) {
	type fields struct {
		ToAdd    []PodToAdd
		ToKeep   []corev1.Pod
		ToRemove []corev1.Pod
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "empty has no changes",
			fields: fields{},
			want:   false,
		},
		{
			name: "something to keep still has no changes",
			fields: fields{
				ToKeep: []corev1.Pod{corev1.Pod{}},
			},
			want: false,
		},
		{
			name: "something to add has changes",
			fields: fields{
				ToAdd: []PodToAdd{PodToAdd{}},
			},
			want: true,
		},
		{
			name: "something to remove has changes",
			fields: fields{
				ToRemove: []corev1.Pod{corev1.Pod{}},
			},
			want: true,
		},
		{
			name: "add and remove has changes",
			fields: fields{
				ToAdd:    []PodToAdd{PodToAdd{}},
				ToRemove: []corev1.Pod{corev1.Pod{}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Changes{
				ToAdd:    tt.fields.ToAdd,
				ToKeep:   tt.fields.ToKeep,
				ToRemove: tt.fields.ToRemove,
			}
			if got := c.HasChanges(); got != tt.want {
				t.Errorf("Changes.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
