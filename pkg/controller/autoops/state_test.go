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
			name:          "empty phase can transition to NoResourcesPhase",
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
			name:          "InvalidPhase should not be overwritten by NoResourcesPhase",
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
			name:          "AutoOpsResourcesNotReadyPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.AutoOpsResourcesNotReadyPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.AutoOpsResourcesNotReadyPhase,
		},
		{
			name:          "MonitoredResourcesNotReadyPhase should be overwritten by AutoOpsResourcesNotReadyPhase",
			initialPhase:  autoopsv1alpha1.MonitoredResourcesNotReadyPhase,
			updatePhase:   autoopsv1alpha1.AutoOpsResourcesNotReadyPhase,
			expectedPhase: autoopsv1alpha1.AutoOpsResourcesNotReadyPhase,
		},
		{
			name:          "AutoOpsResourcesNotReadyPhase should be overwritten by MonitoredResourcesNotReadyPhase",
			initialPhase:  autoopsv1alpha1.AutoOpsResourcesNotReadyPhase,
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

func TestState_CalculateFinalPhase(t *testing.T) {
	tests := []struct {
		name                  string
		initialPhase          autoopsv1alpha1.PolicyPhase
		resources             int
		ready                 int
		isReconciled          bool
		reconciliationMessage string
		expectedPhase         autoopsv1alpha1.PolicyPhase
		expectEvent           bool
	}{
		{
			name:                  "not reconciled should set ApplyingChangesPhase and add event",
			initialPhase:          "",
			resources:             3,
			ready:                 2,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ApplyingChangesPhase,
			expectEvent:           true,
		},
		{
			name:                  "reconciled with all resources ready should set ReadyPhase",
			initialPhase:          "",
			resources:             3,
			ready:                 3,
			isReconciled:          true,
			reconciliationMessage: "",
			expectedPhase:         autoopsv1alpha1.ReadyPhase,
			expectEvent:           false,
		},
		{
			name:                  "reconciled with with not all auto ops resources ready",
			initialPhase:          "",
			resources:             3,
			ready:                 2,
			isReconciled:          true,
			reconciliationMessage: "",
			expectedPhase:         autoopsv1alpha1.AutoOpsResourcesNotReadyPhase,
			expectEvent:           false,
		},
		{
			name:                  "reconciled with ErrorPhase should not be overwritten by ReadyPhase",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 3,
			isReconciled:          true,
			reconciliationMessage: "",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
			expectEvent:           false,
		},
		{
			name:                  "not reconciled with ErrorPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:          autoopsv1alpha1.ErrorPhase,
			resources:             3,
			ready:                 2,
			isReconciled:          false,
			reconciliationMessage: "waiting for resources",
			expectedPhase:         autoopsv1alpha1.ErrorPhase,
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

			state.CalculateFinalPhase(tt.isReconciled, tt.reconciliationMessage)

			assert.Equal(t, tt.expectedPhase, state.status.Phase)
			if tt.expectEvent {
				assert.Len(t, state.Events(), 1, "expected exactly one event")
			} else {
				assert.Empty(t, state.Events(), "expected no events")
			}
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
