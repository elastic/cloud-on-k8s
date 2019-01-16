package mutation

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelper"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestCalculateChanges(t *testing.T) {
	var taintedPod = defaultPod
	taintedPod.Annotations = map[string]string{TaintedAnnotationName: "true"}
	type args struct {
		expected []pod.PodSpecContext
		state    reconcilehelper.ResourcesState
	}
	tests := []struct {
		name string
		args args
		want Changes
	}{
		{
			name: "no changes",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx},
				state:    reconcilehelper.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "2 new pods",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx, defaultPodSpecCtx},
				state:    reconcilehelper.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{
				ToKeep:   []corev1.Pod{defaultPod, defaultPod},
				ToCreate: []PodToCreate{{PodSpecCtx: defaultPodSpecCtx}, {PodSpecCtx: defaultPodSpecCtx}},
			},
		},
		{
			name: "2 less pods",
			args: args{
				expected: []pod.PodSpecContext{},
				state:    reconcilehelper.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
			},
			want: Changes{ToDelete: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "1 pod replaced",
			args: args{
				expected: []pod.PodSpecContext{defaultPodSpecCtx, ESPodSpecContext("another-image", defaultCPULimit)},
				state:    reconcilehelper.ResourcesState{CurrentPods: []corev1.Pod{defaultPod, defaultPod}},
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
				expected: []pod.PodSpecContext{defaultPodSpecCtx, defaultPodSpecCtx},
				state:    reconcilehelper.ResourcesState{CurrentPods: []corev1.Pod{taintedPod, defaultPod}},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod}, ToDelete: []corev1.Pod{defaultPod}, ToCreate: []PodToCreate{PodToCreate{PodSpecCtx: defaultPodSpecCtx}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateChanges(tt.args.expected, tt.args.state, func(ctx pod.PodSpecContext) (corev1.Pod, error) {
				return corev1.Pod{}, nil // TODO: fix
			})
			assert.NoError(t, err)
			assert.Equal(t, len(tt.want.ToKeep), len(got.ToKeep))
			assert.Equal(t, len(tt.want.ToCreate), len(got.ToCreate))
			assert.Equal(t, len(tt.want.ToDelete), len(got.ToDelete))
		})
	}
}
