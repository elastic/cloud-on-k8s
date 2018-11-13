package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var defaultPod = ESPod(defaultNodeData, defaultImage, defaultCPULimit)
var defaultPodSpec = ESPodSpec(defaultNodeData, defaultImage, defaultCPULimit)

func TestCalculateChanges(t *testing.T) {
	type args struct {
		expected []corev1.PodSpec
		actual   []corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want Changes
	}{
		{
			name: "no changes",
			args: args{
				expected: []corev1.PodSpec{defaultPodSpec, defaultPodSpec},
				actual:   []corev1.Pod{defaultPod, defaultPod},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "2 new pods",
			args: args{
				expected: []corev1.PodSpec{defaultPodSpec, defaultPodSpec, defaultPodSpec, defaultPodSpec},
				actual:   []corev1.Pod{defaultPod, defaultPod},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod, defaultPod}, ToAdd: []PodToAdd{PodToAdd{PodSpec: defaultPodSpec}, PodToAdd{PodSpec: defaultPodSpec}}},
		},
		{
			name: "2 less pods",
			args: args{
				expected: []corev1.PodSpec{},
				actual:   []corev1.Pod{defaultPod, defaultPod},
			},
			want: Changes{ToRemove: []corev1.Pod{defaultPod, defaultPod}},
		},
		{
			name: "1 pod replaced",
			args: args{
				expected: []corev1.PodSpec{defaultPodSpec, ESPodSpec(defaultNodeData, "another-image", defaultCPULimit)},
				actual:   []corev1.Pod{defaultPod, defaultPod},
			},
			want: Changes{ToKeep: []corev1.Pod{defaultPod}, ToRemove: []corev1.Pod{defaultPod}, ToAdd: []PodToAdd{PodToAdd{PodSpec: ESPodSpec(defaultNodeData, "another-image", defaultCPULimit)}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateChanges(tt.args.expected, tt.args.actual)
			assert.NoError(t, err)
			assert.Equal(t, len(tt.want.ToKeep), len(got.ToKeep))
			assert.Equal(t, len(tt.want.ToAdd), len(got.ToAdd))
			assert.Equal(t, len(tt.want.ToRemove), len(got.ToRemove))
		})
	}
}
