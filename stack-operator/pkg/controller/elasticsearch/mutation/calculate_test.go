package mutation

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

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
				ToKeep:   []corev1.Pod{defaultPod, defaultPod},
				ToCreate: []PodToCreate{{PodSpecCtx: defaultPodSpecCtx}, {PodSpecCtx: defaultPodSpecCtx}},
			},
		},
		{
			name: "2 less pods",
			args: args{
				expected: []support.PodSpecContext{},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{ToDelete: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "1 pod replaced",
			args: args{
				expected: []support.PodSpecContext{defaultPodSpecCtx, ESPodSpecContext("another-image", defaultCPULimit)},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{
				ToKeep:   []corev1.Pod{defaultPod},
				ToDelete: []corev1.Pod{defaultPod},
				ToCreate: []PodToCreate{{PodSpecCtx: ESPodSpecContext("another-image", defaultCPULimit)}},
			},
		},
		{
			name: "1 pod replaced on pod tainted",
			args: args{
				expected: []support.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx},
				state:    support.ResourcesState{CurrentPods: []corev1.Pod{taintedPod, defaultPod}},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod}, ToDelete: []corev1.Pod{defaultPod}, ToCreate: []PodToCreate{PodToCreate{PodSpecCtx: defaultPodSpecCtx}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateChanges(tt.args.expected, tt.args.state, func(ctx support.PodSpecContext) (corev1.Pod, error) {
				return corev1.Pod{}, nil // TODO: fix
			})
			assert.NoError(t, err)
			assert.Equal(t, len(tt.want.ToKeep), len(got.ToKeep))
			assert.Equal(t, len(tt.want.ToCreate), len(got.ToCreate))
			assert.Equal(t, len(tt.want.ToDelete), len(got.ToDelete))
		})
	}
}
