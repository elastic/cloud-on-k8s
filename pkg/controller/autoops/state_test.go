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
		shouldUpdate  bool
	}{
		{
			name:          "empty phase can transition to ReadyPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.ReadyPhase,
			shouldUpdate:  true,
		},
		{
			name:          "empty phase can transition to ErrorPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.ErrorPhase,
			expectedPhase: autoopsv1alpha1.ErrorPhase,
			shouldUpdate:  true,
		},
		{
			name:          "empty phase can transition to ApplyingChangesPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.ApplyingChangesPhase,
			shouldUpdate:  true,
		},
		{
			name:          "empty phase can transition to InvalidPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.InvalidPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
			shouldUpdate:  true,
		},
		{
			name:          "empty phase can transition to NoResourcesPhase",
			initialPhase:  "",
			updatePhase:   autoopsv1alpha1.NoResourcesPhase,
			expectedPhase: autoopsv1alpha1.NoResourcesPhase,
			shouldUpdate:  true,
		},
		{
			name:          "InvalidPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
			shouldUpdate:  false,
		},
		{
			name:          "InvalidPhase should not be overwritten by ErrorPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.ErrorPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
			shouldUpdate:  false,
		},
		{
			name:          "InvalidPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
			shouldUpdate:  false,
		},
		{
			name:          "InvalidPhase should not be overwritten by NoResourcesPhase",
			initialPhase:  autoopsv1alpha1.InvalidPhase,
			updatePhase:   autoopsv1alpha1.NoResourcesPhase,
			expectedPhase: autoopsv1alpha1.InvalidPhase,
			shouldUpdate:  false,
		},
		{
			name:          "ErrorPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.ErrorPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.ErrorPhase,
			shouldUpdate:  false,
		},
		{
			name:          "NoResourcesPhase should not be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.NoResourcesPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.NoResourcesPhase,
			shouldUpdate:  false,
		},
		{
			name:          "ReadyPhase can be overwritten by ApplyingChangesPhase",
			initialPhase:  autoopsv1alpha1.ReadyPhase,
			updatePhase:   autoopsv1alpha1.ApplyingChangesPhase,
			expectedPhase: autoopsv1alpha1.ApplyingChangesPhase,
			shouldUpdate:  true,
		},
		{
			name:          "ErrorPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.ErrorPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.ErrorPhase,
			shouldUpdate:  false,
		},
		{
			name:          "NoResourcesPhase should not be overwritten by ReadyPhase",
			initialPhase:  autoopsv1alpha1.NoResourcesPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.NoResourcesPhase,
			shouldUpdate:  false,
		},
		{
			name:          "ApplyingChangesPhase can transition to ReadyPhase",
			initialPhase:  autoopsv1alpha1.ApplyingChangesPhase,
			updatePhase:   autoopsv1alpha1.ReadyPhase,
			expectedPhase: autoopsv1alpha1.ReadyPhase,
			shouldUpdate:  true,
		},
		{
			name:          "NoResourcesPhase should not be overwritten by ErrorPhase",
			initialPhase:  autoopsv1alpha1.NoResourcesPhase,
			updatePhase:   autoopsv1alpha1.ErrorPhase,
			expectedPhase: autoopsv1alpha1.NoResourcesPhase,
			shouldUpdate:  false,
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
			state := NewState(policy)
			state.status.Phase = tt.initialPhase
			state.UpdateWithPhase(tt.updatePhase)
			assert.Equal(t, tt.expectedPhase, state.status.Phase)
		})
	}
}
