// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	beatv1b1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
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
			Spec: appsv1.DeploymentSpec{
				Replicas: replicas,
			},
		}
		d.Labels = hash.SetTemplateHashLabel(d.Labels, d.Spec)
		return d
	}
	makeDaemonSet := func(updateStrategy appsv1.DaemonSetUpdateStrategy) *appsv1.DaemonSet {
		d := &appsv1.DaemonSet{
			ObjectMeta: v1.ObjectMeta{Name: resourceName, Namespace: namespace, ResourceVersion: "1"},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: updateStrategy,
			},
		}
		d.Labels = hash.SetTemplateHashLabel(d.Labels, d.Spec)
		return d
	}
	makeStatefulSet := func(replicas *int32) *appsv1.StatefulSet {
		d := &appsv1.StatefulSet{
			ObjectMeta: v1.ObjectMeta{Name: resourceName, Namespace: namespace, ResourceVersion: "1"},
			Spec: appsv1.StatefulSetSpec{
				Replicas: replicas,
			},
		}
		d.Labels = hash.SetTemplateHashLabel(d.Labels, d.Spec)
		return d
	}

	baseKibana := kbv1.Kibana{
		Spec: kbv1.KibanaSpec{
			Version: "9.3.1",
		},
	}
	baseAgent := agentv1alpha1.Agent{
		Spec: agentv1alpha1.AgentSpec{
			Version: "9.3.1",
			StatefulSet: &agentv1alpha1.StatefulSetSpec{
				Replicas: &one,
			},
		}}
	sameUpdateStrategy := appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDaemonSet{
			MaxUnavailable: new(intstr.FromInt32(1)),
		},
	}
	differentUpdateStrategy := appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.OnDeleteDaemonSetStrategyType,
	}
	baseBeat := beatv1b1.Beat{
		Spec: beatv1b1.BeatSpec{
			Version: "9.3.1",
			DaemonSet: &beatv1b1.DaemonSetSpec{
				UpdateStrategy: sameUpdateStrategy,
			},
		},
	}

	tests := []struct {
		name             string
		existingObjs     []client.Object
		parentObject     ObjectWithConditions
		expectedVehicle  client.Object
		clientErr        error
		wantConditionMsg string
		wantEvent        string
		wantError        bool
	}{
		{
			name:             "deployment does not exist — pending changes",
			existingObjs:     nil,
			parentObject:     baseKibana.DeepCopy(),
			expectedVehicle:  makeDeployment(&one),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:             "deployment exists with matching spec — no pending changes",
			existingObjs:     []client.Object{makeDeployment(&one)},
			parentObject:     baseKibana.DeepCopy(),
			expectedVehicle:  makeDeployment(&one),
			wantConditionMsg: PausedNoChangesMessage,
		},
		{
			name:             "deployment exists with different spec — pending changes",
			existingObjs:     []client.Object{makeDeployment(&one)},
			parentObject:     baseKibana.DeepCopy(),
			expectedVehicle:  makeDeployment(&two),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:             "statefulset does not exist — pending changes",
			existingObjs:     nil,
			parentObject:     baseAgent.DeepCopy(),
			expectedVehicle:  makeStatefulSet(&one),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:             "statefulset exists with matching spec — no pending changes",
			existingObjs:     []client.Object{makeStatefulSet(&one)},
			parentObject:     baseAgent.DeepCopy(),
			expectedVehicle:  makeStatefulSet(&one),
			wantConditionMsg: PausedNoChangesMessage,
		},
		{
			name:             "statefulset exists with different spec — pending changes",
			existingObjs:     []client.Object{makeStatefulSet(&one)},
			parentObject:     baseAgent.DeepCopy(),
			expectedVehicle:  makeStatefulSet(&two),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:             "daemonset does not exist — pending changes",
			existingObjs:     nil,
			parentObject:     baseBeat.DeepCopy(),
			expectedVehicle:  makeDaemonSet(sameUpdateStrategy),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:             "daemonset exists with matching spec — no pending changes",
			existingObjs:     []client.Object{makeDaemonSet(sameUpdateStrategy)},
			parentObject:     baseBeat.DeepCopy(),
			expectedVehicle:  makeDaemonSet(sameUpdateStrategy),
			wantConditionMsg: PausedNoChangesMessage,
		},
		{
			name:             "daemonset exists with different spec — pending changes",
			existingObjs:     []client.Object{makeDeployment(&one)},
			parentObject:     baseBeat.DeepCopy(),
			expectedVehicle:  makeDaemonSet(differentUpdateStrategy),
			wantConditionMsg: PausedWithPendingChangesMessage,
			wantEvent:        "Warning Paused " + PausedWithPendingChangesMessage,
		},
		{
			name:            "client error — returns error, no condition set",
			parentObject:    baseKibana.DeepCopy(),
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

			err := SetPausedConditionAndEmitEvent(context.Background(), c, recorder, tt.parentObject, tt.expectedVehicle)

			if tt.wantError {
				assert.Error(t, err)
				assert.Empty(t, tt.parentObject.Conditions(), "no condition should be set on error")
				return
			}

			require.NoError(t, err)

			conditions := tt.parentObject.Conditions()
			idx := conditions.Index(commonv1.OrchestrationPaused)
			require.GreaterOrEqual(t, idx, 0, "OrchestrationPaused condition should be present")
			assert.Equal(t, corev1.ConditionTrue, conditions[idx].Status)
			assert.Equal(t, tt.wantConditionMsg, conditions[idx].Message)

			select {
			case event := <-recorder.Events:
				assert.Equal(t, tt.wantEvent, event)
			default:
				require.Empty(t, tt.wantEvent, "event should be written to events channel when tt.wantEvent is non-empty")
				assert.Empty(t, recorder.Events, "received unexpected event")
			}
		})
	}
}

func Test_hasPendingChanges(t *testing.T) {
	originalDeployment := appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: fmt.Sprintf("foo-%d", 1),
						},
					},
				},
			},
		},
	}
	originalDeployment = deployment.WithTemplateHash(originalDeployment)

	originalDaemonSet := appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: fmt.Sprintf("foo-%d", 1),
						},
					},
				},
			},
		},
	}
	originalDaemonSet = daemonset.WithTemplateHash(originalDaemonSet)

	originalStatefulSet := appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: fmt.Sprintf("foo-%d", 1),
						},
					},
				},
			},
		},
	}
	originalStatefulSet = statefulset.WithTemplateHash(originalStatefulSet)

	tests := []struct {
		name             string
		existing         client.Object
		expectedObject   client.Object
		expectHasChanged bool
		expectedError    error
	}{
		{
			name: "deployment exists and has different hash label returns true",
			existing: func() *appsv1.Deployment {
				existing := originalDeployment.DeepCopy()
				*existing = deployment.WithTemplateHash(*existing)
				return existing
			}(),
			expectedObject: func() *appsv1.Deployment {
				updated := originalDeployment.DeepCopy()
				updated.Spec.Template.Labels["kibana.k8s.elastic.co/version"] = "9.4.0"
				*updated = deployment.WithTemplateHash(*updated)
				return updated
			}(),
			expectHasChanged: true,
		},
		{
			name:             "deployment exists and has the same hash label returns false",
			existing:         originalDeployment.DeepCopy(),
			expectedObject:   originalDeployment.DeepCopy(),
			expectHasChanged: false,
		},
		{
			name:             "deployment does not exist yet returns true",
			existing:         nil,
			expectedObject:   originalDeployment.DeepCopy(),
			expectHasChanged: true,
		},
		{
			name: "daemonset exists and has different hash label returns true",
			existing: func() *appsv1.DaemonSet {
				existing := originalDaemonSet.DeepCopy()
				*existing = daemonset.WithTemplateHash(*existing)
				return existing
			}(),
			expectedObject: func() *appsv1.DaemonSet {
				updated := originalDaemonSet.DeepCopy()
				updated.Spec.Template.Labels["kibana.k8s.elastic.co/version"] = "9.4.0"
				*updated = daemonset.WithTemplateHash(*updated)
				return updated
			}(),
			expectHasChanged: true,
		},
		{
			name:             "daemonset exists and has the same hash label returns false",
			existing:         originalDaemonSet.DeepCopy(),
			expectedObject:   originalDaemonSet.DeepCopy(),
			expectHasChanged: false,
		},
		{
			name:             "daemonset does not exist yet returns true",
			existing:         nil,
			expectedObject:   originalDaemonSet.DeepCopy(),
			expectHasChanged: true,
		},
		{
			name: "statefulset exists and has different hash label returns true",
			existing: func() *appsv1.StatefulSet {
				existing := originalStatefulSet.DeepCopy()
				*existing = statefulset.WithTemplateHash(*existing)
				return existing
			}(),
			expectedObject: func() *appsv1.StatefulSet {
				updated := originalStatefulSet.DeepCopy()
				updated.Spec.Template.Labels["kibana.k8s.elastic.co/version"] = "9.4.0"
				*updated = statefulset.WithTemplateHash(*updated)
				return updated
			}(),
			expectHasChanged: true,
		},
		{
			name:             "statefulset exists and has the same hash label returns false",
			existing:         originalStatefulSet.DeepCopy(),
			expectedObject:   originalStatefulSet.DeepCopy(),
			expectHasChanged: false,
		},
		{
			name:             "statefulset does not exist yet returns true",
			existing:         nil,
			expectedObject:   originalStatefulSet.DeepCopy(),
			expectHasChanged: true,
		},
	}

	for _, tt := range tests {
		var c k8s.Client
		switch {
		case tt.expectedError != nil:
			c = k8s.NewFailingClient(tt.expectedError)
		case tt.existing != nil:
			c = k8s.NewFakeClient(tt.existing)
		default:
			c = k8s.NewFakeClient()
		}

		hasChanged, _ := hasPendingChanges(context.Background(), c, tt.expectedObject)
		assert.Equal(t, tt.expectHasChanged, hasChanged)
	}
}

func TestMaybeResetPausedCondition(t *testing.T) {
	tests := []struct {
		name                string
		parent              ObjectWithConditions
		wantConditionExists bool
		wantStatus          corev1.ConditionStatus
		wantMessage         string
		wantEvent           string
	}{
		{
			name:                "empty conditions — no-op, condition not added",
			parent:              &mockWithConditions{conditions: commonv1.Conditions{}},
			wantConditionExists: false,
		},
		{
			name: "OrchestrationPaused=True — resets to False",
			parent: &mockWithConditions{conditions: commonv1.Conditions{
				{Type: commonv1.OrchestrationPaused, Status: corev1.ConditionTrue, Message: "Orchestration is paused"},
			}},
			wantConditionExists: true,
			wantStatus:          corev1.ConditionFalse,
			wantMessage:         PausedOrchestrationResumed,
			wantEvent:           "Normal Resumed Orchestration has resumed normally",
		},
		{
			name: "OrchestrationPaused=False already — idempotent, stays False",
			parent: &mockWithConditions{conditions: commonv1.Conditions{
				{Type: commonv1.OrchestrationPaused, Status: corev1.ConditionFalse, Message: PausedOrchestrationResumed},
			}},
			wantConditionExists: true,
			wantStatus:          corev1.ConditionFalse,
			wantMessage:         PausedOrchestrationResumed,
		},
		{
			name: "unrelated condition present — OrchestrationPaused not added",
			parent: &mockWithConditions{conditions: commonv1.Conditions{
				{Type: "SomeOtherCondition", Status: corev1.ConditionTrue},
			}},
			wantConditionExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := toolsevents.NewFakeRecorder(1)
			MaybeResetPausedCondition(recorder, tt.parent)

			conditions := tt.parent.Conditions()
			idx := conditions.Index(commonv1.OrchestrationPaused)
			if !tt.wantConditionExists {
				assert.Equal(t, -1, idx, "OrchestrationPaused condition should not be present")
				return
			}
			require.GreaterOrEqual(t, idx, 0, "OrchestrationPaused condition should be present")
			assert.Equal(t, tt.wantStatus, conditions[idx].Status)
			assert.Equal(t, tt.wantMessage, conditions[idx].Message)

			select {
			case event := <-recorder.Events:
				assert.Equal(t, tt.wantEvent, event)
			default:
				require.Empty(t, tt.wantEvent, "event should be written to events channel when tt.wantEvent is non-empty")
				assert.Empty(t, recorder.Events, "received unexpected event")
			}
		})
	}
}

type mockWithConditions struct {
	client.Object
	conditions commonv1.Conditions
}

func (m *mockWithConditions) MergeConditions(cs ...commonv1.Condition) {
	m.conditions = m.conditions.MergeWith(cs...)
}

func (m *mockWithConditions) Conditions() commonv1.Conditions {
	return m.conditions
}
