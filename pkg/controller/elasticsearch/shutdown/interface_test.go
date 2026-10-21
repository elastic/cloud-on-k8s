// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shutdown

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeShutdowns struct {
	Interface
	reconcileErr error
}

func (f *fakeShutdowns) ReconcileShutdowns(context.Context, []string) error {
	return f.reconcileErr
}

type recordingObserver struct {
	Observer
	// reported are the nodes reported in progress
	reported []string
}

func (r *recordingObserver) OnReconcileShutdowns(leavingNodes []string) {
	r.reported = append(r.reported, leavingNodes...)
}

func TestObserved_ReconcileShutdowns(t *testing.T) {
	failure := errors.New("failure")
	tests := []struct {
		name           string
		implementation *fakeShutdowns
		// wantReported are the nodes reported in progress
		wantReported []string
		wantErr      error
	}{
		{
			name:           "reports all the nodes on success",
			implementation: &fakeShutdowns{},
			wantReported:   []string{"pod-1", "pod-2", "pod-3"},
		},
		{
			name:           "reports no node on failure",
			implementation: &fakeShutdowns{reconcileErr: failure},
			wantErr:        failure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer := &recordingObserver{}
			err := WithObserver(tt.implementation, observer).
				ReconcileShutdowns(context.Background(), []string{"pod-1", "pod-2", "pod-3"})
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantReported, observer.reported)
		})
	}
}

func TestObserved_ReconcileShutdowns_noObserver(t *testing.T) {
	failure := errors.New("failure")
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "success", err: nil, wantErr: nil},
		{name: "failure", err: failure, wantErr: failure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WithObserver(&fakeShutdowns{reconcileErr: tt.err}, nil).
				ReconcileShutdowns(t.Context(), []string{"pod-1"})
			assert.Equal(t, tt.wantErr, err)
		})
	}
}
