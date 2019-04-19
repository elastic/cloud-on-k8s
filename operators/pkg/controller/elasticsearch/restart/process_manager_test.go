// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"errors"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
)

func Test_ensureESProcessStopped(t *testing.T) {
	tests := []struct {
		name     string
		pmClient processmanager.Client
		podName  string
		want     bool
		wantErr  bool
	}{
		{
			name:     "node stopped",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopped}, nil),
			podName:  "pod",
			want:     true,
			wantErr:  false,
		},
		{
			name:     "node started",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil),
			podName:  "pod",
			want:     false,
			wantErr:  false,
		},
		{
			name:     "node stopping",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopping}, nil),
			podName:  "pod",
			want:     false,
			wantErr:  false,
		},
		{
			name:     "process manager unavailable",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopping}, errors.New("failure")),
			podName:  "pod",
			want:     false,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ensureESProcessStopped(tt.pmClient, tt.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ensureESProcessStopped() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ensureESProcessStopped() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ensureESProcessStarted(t *testing.T) {
	tests := []struct {
		name     string
		pmClient processmanager.Client
		podName  string
		want     bool
		wantErr  bool
	}{
		{
			name:     "node stopped",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopped}, nil),
			podName:  "pod",
			want:     false,
			wantErr:  false,
		},
		{
			name:     "node started",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil),
			podName:  "pod",
			want:     true,
			wantErr:  false,
		},
		{
			name:     "node starting",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopping}, nil),
			podName:  "pod",
			want:     false,
			wantErr:  false,
		},
		{
			name:     "process manager unavailable",
			pmClient: processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopping}, errors.New("failure")),
			podName:  "pod",
			want:     false,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ensureESProcessStarted(tt.pmClient, tt.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ensureESProcessStopped() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ensureESProcessStopped() = %v, want %v", got, tt.want)
			}
		})
	}
}
