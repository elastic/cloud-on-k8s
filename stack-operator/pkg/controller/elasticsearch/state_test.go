package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNodesAvailable(t *testing.T) {
	tests := []struct {
		input    []corev1.Pod
		expected int
	}{
		{
			input: []corev1.Pod{
				corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							corev1.PodCondition{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							corev1.PodCondition{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							corev1.PodCondition{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							corev1.PodCondition{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: 1,
		},
		{
			input: []corev1.Pod{
				corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							corev1.PodCondition{
								Type:   corev1.PodScheduled,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							corev1.PodCondition{
								Type:   corev1.PodScheduled,
								Status: corev1.ConditionTrue,
							},
							corev1.PodCondition{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
							corev1.PodCondition{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: 0,
		},
		{
			input: []corev1.Pod{
				corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							corev1.PodCondition{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							corev1.PodCondition{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							corev1.PodCondition{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							corev1.PodCondition{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, len(AvailableElasticsearchNodes(tt.input)))
	}
}
