package mutation

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPerformableChanges_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		changes PerformableChanges
		want    bool
	}{
		{name: "empty", changes: PerformableChanges{}, want: true},
		{name: "creation", changes: PerformableChanges{ScheduleForCreation: []CreatablePod{{}}}, want: false},
		{name: "deletion", changes: PerformableChanges{ScheduleForDeletion: []corev1.Pod{{}}}, want: false},
		{
			name: "creation and deletion",
			changes: PerformableChanges{
				ScheduleForCreation: []CreatablePod{{}},
				ScheduleForDeletion: []corev1.Pod{{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.changes
			if got := c.IsEmpty(); got != tt.want {
				t.Errorf("PerformableChanges.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
