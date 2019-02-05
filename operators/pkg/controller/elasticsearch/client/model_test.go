package client

import (
	"testing"
	"time"
)

func TestSnapshot_EndedBefore(t *testing.T) {
	now := time.Date(2018, 11, 17, 0, 9, 0, 0, time.UTC)
	tests := []struct {
		name   string
		fields time.Time
		args   time.Duration
		want   bool
	}{
		{
			name:   "no end time is possible",
			fields: time.Time{},
			args:   1 * time.Hour,
			want:   false,
		},
		{
			name:   "one hour is less than 2 hours",
			fields: now.Add(-2 * time.Hour),
			args:   1 * time.Hour,
			want:   true,
		},
		{
			name:   "one hour is more than 30 minutes",
			fields: now.Add(-30 * time.Minute),
			args:   1 * time.Hour,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Snapshot{
				EndTime: tt.fields,
			}
			if got := s.EndedBefore(tt.args, now); got != tt.want {
				t.Errorf("Snapshot.EndedBefore() = %v, want %v", got, tt.want)
			}
		})
	}
}
