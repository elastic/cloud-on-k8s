// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestModel_RemoteCluster(t *testing.T) {
	tests := []struct {
		name string
		arg  Settings
		want string
	}{
		{
			name: "Simple remote cluster",
			arg: Settings{
				PersistentSettings: SettingGroup{
					RemoteClusters: map[string]RemoteCluster{
						"leader": {
							Seeds: []string{"127.0.0.1:9300"},
						},
					},
				},
			},
			want: `{"persistent":{"cluster.remote":{"leader":{"seeds":["127.0.0.1:9300"]}}},"transient":{}}`,
		},
		{
			name: "Deleted remote cluster",
			arg: Settings{
				PersistentSettings: SettingGroup{
					RemoteClusters: map[string]RemoteCluster{
						"leader": {
							Seeds: nil,
						},
					},
				},
			},
			want: `{"persistent":{"cluster.remote":{"leader":{"seeds":null}}},"transient":{}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json, err := json.Marshal(tt.arg)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, string(json))
		})
	}
}
