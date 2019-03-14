package driver

import (
	"testing"
	"time"
)

func Test_shouldRequeueIn(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name                       string
		now                        time.Time
		certExpiration             time.Time
		certExpirationSafetyMargin time.Duration
		want                       time.Duration
	}{
		{
			name:                       "requeue in less than a year",
			now:                        now,
			certExpiration:             now.Add(365 * 24 * time.Hour),
			certExpirationSafetyMargin: 24 * time.Hour,
			want:                       364*24*time.Hour + 1*time.Second,
		},
		{
			name:                       "requeue in less than 10 hours",
			now:                        now,
			certExpiration:             now.Add(10 * time.Hour),
			certExpirationSafetyMargin: 1 * time.Hour,
			want:                       9*time.Hour + 1*time.Second,
		},
		{
			name:                       "requeue asap, we're in the safety margin already",
			now:                        now,
			certExpiration:             now.Add(10 * time.Hour),
			certExpirationSafetyMargin: 20 * time.Hour,
			want:                       0 * time.Second,
		},
		{
			name:                       "cert already expired, requeue asap",
			now:                        now,
			certExpiration:             now.Add(-1 * time.Hour),
			certExpirationSafetyMargin: 10 * time.Hour,
			want:                       0 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRequeueIn(tt.now, tt.certExpiration, tt.certExpirationSafetyMargin); got != tt.want {
				t.Errorf("shouldRequeueIn() = %v, want %v", got, tt.want)
			}
		})
	}
}
