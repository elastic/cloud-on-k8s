// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"
	"time"
)

func Test_shouldRequeueIn(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name               string
		now                time.Time
		certExpiration     time.Time
		caCertRotateBefore time.Duration
		want               time.Duration
	}{
		{
			name:               "requeue in less than a year",
			now:                now,
			certExpiration:     now.Add(365 * 24 * time.Hour),
			caCertRotateBefore: 24 * time.Hour,
			want:               364*24*time.Hour + 1*time.Second,
		},
		{
			name:               "requeue in less than 10 hours",
			now:                now,
			certExpiration:     now.Add(10 * time.Hour),
			caCertRotateBefore: 1 * time.Hour,
			want:               9*time.Hour + 1*time.Second,
		},
		{
			name:               "requeue asap, we're in the safety margin already",
			now:                now,
			certExpiration:     now.Add(10 * time.Hour),
			caCertRotateBefore: 20 * time.Hour,
			want:               0 * time.Second,
		},
		{
			name:               "cert already expired, requeue asap",
			now:                now,
			certExpiration:     now.Add(-1 * time.Hour),
			caCertRotateBefore: 10 * time.Hour,
			want:               0 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRequeueIn(tt.now, tt.certExpiration, tt.caCertRotateBefore); got != tt.want {
				t.Errorf("shouldRequeueIn() = %v, want %v", got, tt.want)
			}
		})
	}
}
