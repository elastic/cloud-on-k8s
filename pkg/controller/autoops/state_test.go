// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
)

func TestState_UpdateWithPhase(t *testing.T) {
	tests := []struct {
		name          string
		initialPhase  autoopsv1alpha1.PolicyPhase
		updatePhase   autoopsv1alpha1.PolicyPhase
		expectedPhase autoopsv1alpha1.PolicyPhase
	}{
		{
			name:          "empty phase can transition to ReadyPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.ReadyPhase,
		},
		{
			name:          "empty phase can transition to ErrorPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.ErrorPhase,
			expectedPhase: autoopsv1alpha1.ErrorPhase,
		},
		{
			name:          "empty phase can transition to ApplyingChangesPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.ApplyingChangesPhase,
		},
		{
			name:          "empty phase can transition to InvalidPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.InvalidPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
		},
		{
			name:          "empty phase can transition to NoMonitoredResourcesPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.NoMonitoredResourcesPhase,
			expectedPhase: autoopsv1alpha1.NoMonitoredResourcesPhase,
		},
		{
			name:          "InvalidPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
		},
		{
			name:          "InvalidPhase should not be overwritten by ErrorPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.ErrorPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
		},
		{
			name:          "InvalidPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
		},
		{
			name:          "InvalidPhase should not be overwritten by NoMonitoredResourcesPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.NoMonitoredResourcesPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
		},
		{
			name:          "ErrorPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.ErrorPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.ErrorPhase,
		},
		{
			name:          "NoResourcesPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.NoMonitoredResourcesPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.NoMonitoredResourcesPhase,
		},
		{
			name:          "ReadyPhase can be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.ReadyPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.ApplyingChangesPhase,
		},
		{
			name:          "ErrorPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.ErrorPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.ErrorPhase,
		},
		{
			name:          "NoResourcesPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.NoMonitoredResourcesPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.NoMonitoredResourcesPhase,
		},
		{
			name:          "ApplyingChangesPhase can transition to ReadyPhase",
			initialPhase:  autoopsv1alpha1.ApplyingChangesPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.ReadyPhase,
		},
		{
			name:          "NoResourcesPhase should not be overwritten by ErrorPhase",
			initialPhase:  autoopsv1alpha1.NoMonitoredResourcesPhase,
			updatePhase:   autoopsv1alpha1.ErrorPhase,
			expectedPhase: autoopsv1alpha1.NoMonitoredResourcesPhase,
		},
		{
			name:          "AutoOpsAgentsNotReadyPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.AutoOpsAgentsNotReadyPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.AutoOpsAgentsNotReadyPhase,
		},
		{
			name:          "MonitoredResourcesNotReadyPhase should be overwritten by AutoOpsAgentsNotReadyPhase",
			initialPhase:  autoopsv1alpha1.MonitoredResourcesNotReadyPhase,
			updatePhase:   autoopsv1alpha1.AutoOpsAgentsNotReadyPhase,
			expectedPhase: autoopsv1alpha1.AutoOpsAgentsNotReadyPhase,
		},
		{
			name:          "AutoOpsAgentsNotReadyPhase should be overwritten by MonitoredResourcesNotReadyPhase",
			initialPhase:  autoopsv1alpha1.AutoOpsAgentsNotReadyPhase,
			updatePhase:   autoopsv1alpha1.MonitoredResourcesNotReadyPhase,
			expectedPhase: autoopsv1alpha1.MonitoredResourcesNotReadyPhase,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
				},
				Status: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
					Phase: tt.initialPhase,
				},
			}
			state := newState(policy)
			state.status.Phase = tt.initialPhase
			state.UpdateWithPhase(tt.updatePhase)
			assert.Equal(t, tt.expectedPhase, state.status.Phase)
		})
	}
}

func TestState_Finalize(t *testing.T) {
	tests := []struct {
		name                  string
		initialPhase          autoopsv1alpha1.PolicyPhase
		resources             int
		ready                 int
		errors                int
		skipped               int
		isReconciled          bool
		reconciliationMessage string
		expectedPhase         autoopsv1alpha1.PolicyPhase
		expectedReadyCount    string
		expectEvent           bool
	}{
		{
			name:                  "not reconciled should set ApplyingChangesPhase and add event",
			initialPhase:          "",
			resources:             3,
			ready:                 2,
			errors:                0,
			skipped:               0,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ApplyingChangesPhase,
			expectedReadyCount:    "2/3",
			expectEvent:           true,
		},
		{
			name:                  "reconciled with all resources ready should set ReadyPhase",
			initialPhase:          "",
			resources:             3,
			ready:                 3,
			errors:                0,
			skipped:               0,
			isReconciled:          true,
			reconciliationMessage: "",
			expectedPhase:         autoopsv1alpha1.ReadyPhase,
			expectedReadyCount:    "3/3",
			expectEvent:           false,
		},
		{
			name:                  "reconciled with with not all auto ops resources ready",
			initialPhase:          "",
			resources:             3,
			ready:                 2,
			errors:                0,
			skipped:               0,
			isReconciled:          true,
			reconciliationMessage: "",
			expectedPhase:         autoopsv1alpha1.AutoOpsAgentsNotReadyPhase,
			expectedReadyCount:    "2/3",
			expectEvent:           false,
		},
		{
			name:                  "reconciled with ErrorPhase should not be overwritten by ReadyPhase",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 3,
			errors:                0,
			skipped:               0,
			isReconciled:          true,
			reconciliationMessage: "",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
			expectedReadyCount:    "3/3",
			expectEvent:           false,
		},
		{
			name:                  "not reconciled with ErrorPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 2,
			errors:                0,
			skipped:               0,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
			expectedReadyCount:    "2/3",
			expectEvent:           true,
		},
		{
			name:                  "errors and skipped",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 1,
			errors:                1,
			skipped:               1,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
			expectedReadyCount:    "1/3",
			expectEvent:           true,
		},
		{
			name:                  "skipped",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 1,
			errors:                0,
			skipped:               2,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
			expectedReadyCount:    "1/3",
			expectEvent:           true,
		},
		{
			name:                  "error",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 1,
			errors:                2,
			skipped:               0,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
			expectedReadyCount:    "1/3",
			expectEvent:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
				},
				Status: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
					Phase: tt.initialPhase,
				},
			}
			state := newState(policy)
			state.status.Phase = tt.initialPhase
			state.status.Resources = tt.resources
			state.status.Ready = tt.ready
			state.status.Errors = tt.errors
			state.status.Skipped = tt.skipped

			state.Finalize(tt.isReconciled, tt.reconciliationMessage)

			assert.Equal(t, tt.expectedPhase, state.status.Phase)
			if tt.expectEvent {
				assert.Len(t, state.Events(), 1, "expected exactly one event")
			} else {
				assert.Empty(t, state.Events(), "expected no events")
			}

			assert.Equal(t, tt.expectedReadyCount, state.status.ReadyCount)
		})
	}
}

func TestState_UpdateResources(t *testing.T) {
	t.Run("count > 0 sets resources without changing phase", func(t *testing.T) {
		state := newState(autoopsv1alpha1.AutoOpsAgentPolicy{})
		state.UpdateMonitoredResources(5)
		assert.Equal(t, 5, state.status.Resources)
		assert.Equal(t, autoopsv1alpha1.PolicyPhase(""), state.status.Phase)
	})

	t.Run("count == 0 sets NoResourcesPhase", func(t *testing.T) {
		state := newState(autoopsv1alpha1.AutoOpsAgentPolicy{})
		state.UpdateMonitoredResources(0)
		assert.Equal(t, 0, state.status.Resources)
		assert.Equal(t, autoopsv1alpha1.NoMonitoredResourcesPhase, state.status.Phase)
	})
}
