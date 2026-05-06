// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

type testcase struct {
	name string

	// annotationSequence is list of annotations that are simulated.
	annotationSequence []map[string]string

	// Expected (un)managed state.
	expectedState []bool
}

func TestIsOrchestrationPaused(t *testing.T) {
	tests := []testcase{
		{
			name: "Simple paused-orchestration simulation (a.k.a the Happy Path)",
			annotationSequence: []map[string]string{
				{PauseOrchestrationAnnotation: "true"},
				{PauseOrchestrationAnnotation: "false"},
				{PauseOrchestrationAnnotation: "true"},
				{PauseOrchestrationAnnotation: "false"},
			},
			expectedState: []bool{
				true,
				false,
				true,
				false,
			},
		},
		{
			name: "Only 'true' means paused",
			annotationSequence: []map[string]string{
				{PauseOrchestrationAnnotation: ""}, // empty annotation
				{PauseOrchestrationAnnotation: "true"},
				{PauseOrchestrationAnnotation: "XXXX"}, // unable to parse these
				{PauseOrchestrationAnnotation: "1"},
				{PauseOrchestrationAnnotation: "0"},
			},
			expectedState: []bool{
				false,
				true,
				false,
				false,
				false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i, expectedState := range test.expectedState {
				// testing with a secret, but could be any kind
				obj := corev1.Secret{ObjectMeta: v1.ObjectMeta{
					Name:        "bar",
					Namespace:   "foo",
					Annotations: test.annotationSequence[i],
				}}
				actualPauseState := IsOrchestrationPaused(&obj)
				assert.Equal(t, expectedState, actualPauseState, test.annotationSequence[i])
			}
		})
	}
}

func TestSetPausedConditionAndEmitEvent(t *testing.T) {
	const (
		resourceName = "test-resource"
		namespace    = "default"
	)

	// makeDeployment builds a deployment whose spec contains the given replica count,
	// simulating what deployment.New produces (no template hash label pre-set).
	one := int32(1)
	two := int32(2)
	makeDeployment := func(replicas *int32) *appsv1.Deployment {
		d := &appsv1.Deployment{
			ObjectMeta: v1.ObjectMeta{Name: resourceName, Namespace: namespace, ResourceVersion: "1"},
			Spec:       appsv1.DeploymentSpec{Replicas: replicas},
		}
		d.Labels = hash.SetTemplateHashLabel(d.Labels, *d)
		return d
	}
	owner := &corev1.ConfigMap{ObjectMeta: v1.ObjectMeta{Name: "owner", Namespace: namespace}}

	tests := []struct {
		name             string
		existingObjs     []client.Object
		expectedVehicle  *appsv1.Deployment
		clientErr        error
		wantConditionMsg string
		wantEvent        string
		wantError        bool
	}{
		{
			name:             "resource does not exist — pending changes",
			existingObjs:     nil,
			expectedVehicle:  makeDeployment(&one),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:             "resource exists with matching spec — no pending changes",
			existingObjs:     []client.Object{makeDeployment(&one)},
			expectedVehicle:  makeDeployment(&one),
			wantConditionMsg: PausedNoChangesMessage,
			wantEvent:        "Warning Paused " + PausedNoChangesMessage,
		},
		{
			name:             "resource exists with different spec — pending changes",
			existingObjs:     []client.Object{makeDeployment(&one)},
			expectedVehicle:  makeDeployment(&two),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:            "client error — returns error, no condition set",
			clientErr:       errors.New("connection refused"),
			expectedVehicle: makeDeployment(&one),
			wantError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c k8s.Client
			if tt.clientErr != nil {
				c = k8s.NewFailingClient(tt.clientErr)
			} else {
				c = k8s.NewFakeClient(tt.existingObjs...)
			}
			recorder := toolsevents.NewFakeRecorder(10)
			conditions := commonv1.Conditions{}

			err := SetPausedConditionAndEmitEvent(context.Background(), c, recorder, owner, tt.expectedVehicle, &conditions)

			if tt.wantError {
				assert.Error(t, err)
				assert.Empty(t, conditions, "no condition should be set on error")
				return
			}

			require.NoError(t, err)

			idx := conditions.Index(commonv1.OrchestrationPaused)
			require.GreaterOrEqual(t, idx, 0, "OrchestrationPaused condition should be present")
			assert.Equal(t, corev1.ConditionTrue, conditions[idx].Status)
			assert.Equal(t, tt.wantConditionMsg, conditions[idx].Message)

			select {
			case event := <-recorder.Events:
				assert.Equal(t, tt.wantEvent, event)
			default:
				t.Error("expected a warning event to be emitted but none was")
			}
		})
	}
}

func TestMaybeResetPausedCondition(t *testing.T) {
	tests := []struct {
		name                string
		initialConditions   commonv1.Conditions
		wantConditionExists bool
		wantStatus          corev1.ConditionStatus
		wantMessage         string
	}{
		{
			name:                "empty conditions — no-op, condition not added",
			initialConditions:   commonv1.Conditions{},
			wantConditionExists: false,
		},
		{
			name: "OrchestrationPaused=True — resets to False",
			initialConditions: commonv1.Conditions{
				{Type: commonv1.OrchestrationPaused, Status: corev1.ConditionTrue, Message: "Orchestration is paused"},
			},
			wantConditionExists: true,
			wantStatus:          corev1.ConditionFalse,
			wantMessage:         PausedOrchestrationResumed,
		},
		{
			name: "OrchestrationPaused=False already — idempotent, stays False",
			initialConditions: commonv1.Conditions{
				{Type: commonv1.OrchestrationPaused, Status: corev1.ConditionFalse, Message: PausedOrchestrationResumed},
			},
			wantConditionExists: true,
			wantStatus:          corev1.ConditionFalse,
			wantMessage:         PausedOrchestrationResumed,
		},
		{
			name: "unrelated condition present — OrchestrationPaused not added",
			initialConditions: commonv1.Conditions{
				{Type: "SomeOtherCondition", Status: corev1.ConditionTrue},
			},
			wantConditionExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := tt.initialConditions.DeepCopy()
			MaybeResetPausedCondition(&conditions)

			idx := conditions.Index(commonv1.OrchestrationPaused)
			if !tt.wantConditionExists {
				assert.Equal(t, -1, idx, "OrchestrationPaused condition should not be present")
				return
			}
			require.GreaterOrEqual(t, idx, 0, "OrchestrationPaused condition should be present")
			assert.Equal(t, tt.wantStatus, conditions[idx].Status)
			assert.Equal(t, tt.wantMessage, conditions[idx].Message)
		})
	}
}
