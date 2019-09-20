// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"reflect"
	"testing"
	"time"

	v1alpha12 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNodesAvailable(t *testing.T) {
	tests := []struct {
		input    []corev1.Pod
		expected int
	}{
		{
			input: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: 1,
		},
		{
			input: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodScheduled,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodScheduled,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: 0,
		},
		{
			input: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, len(AvailableElasticsearchNodes(tt.input)))
	}
}

func TestState_Apply(t *testing.T) {
	tests := []struct {
		name       string
		cluster    v1alpha1.Elasticsearch
		effects    func(s *State)
		wantEvents []events.Event
		wantStatus *v1alpha1.ElasticsearchStatus
	}{
		{
			name:       "defaults",
			cluster:    v1alpha1.Elasticsearch{},
			wantEvents: []events.Event{},
			wantStatus: nil,
		},
		{
			name:    "no degraded health event on cluster formation",
			cluster: v1alpha1.Elasticsearch{},
			effects: func(s *State) {
				s.UpdateElasticsearchApplyingChanges([]corev1.Pod{})
			},
			wantEvents: []events.Event{},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health: v1alpha1.ElasticsearchRedHealth,
				Phase:  v1alpha1.ElasticsearchApplyingChangesPhase,
			},
		},
		{
			name: "no degraded health event when cluster info is unknown",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health: v1alpha1.ElasticsearchGreenHealth,
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchState(ResourcesState{}, observer.State{
					ClusterHealth: nil,
					ClusterInfo:   nil,
				})
			},
			wantEvents: []events.Event{},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health: "unknown",
				Phase:  "",
			},
		},
		{
			name: "health degraded",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health: v1alpha1.ElasticsearchGreenHealth,
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchApplyingChanges([]corev1.Pod{})
			},
			wantEvents: []events.Event{{EventType: corev1.EventTypeWarning, Reason: events.EventReasonUnhealthy, Message: "Elasticsearch cluster health degraded"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health: v1alpha1.ElasticsearchRedHealth,
				Phase:  v1alpha1.ElasticsearchApplyingChangesPhase,
			},
		},
		{
			name: "cluster state lost",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health:      v1alpha1.ElasticsearchRedHealth,
					ClusterUUID: "old",
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchReady(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterInfo: &client.Info{
						ClusterUUID: "new",
					},
				})
			},
			wantEvents: []events.Event{{EventType: corev1.EventTypeWarning, Reason: events.EventReasonUnexpected, Message: "Cluster UUID changed (was: old, is: new)"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:      v1alpha1.ElasticsearchRedHealth,
				Phase:       v1alpha1.ElasticsearchReadyPhase,
				ClusterUUID: "new",
			},
		},
		{
			name: "no UUID change event on cluster formation",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					ClusterUUID: "",
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchReady(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterInfo: &client.Info{
						ClusterUUID: "new",
					},
				})
			},
			wantEvents: []events.Event{},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:      v1alpha1.ElasticsearchRedHealth,
				Phase:       v1alpha1.ElasticsearchReadyPhase,
				ClusterUUID: "new",
			},
		},
		{
			name: "Ignore temporary cluster downtime",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health:      v1alpha1.ElasticsearchRedHealth,
					ClusterUUID: "old",
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchReady(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterInfo: &client.Info{
						ClusterUUID: "",
					},
				})
			},
			wantEvents: []events.Event{},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:      v1alpha1.ElasticsearchRedHealth,
				Phase:       v1alpha1.ElasticsearchReadyPhase,
				ClusterUUID: "old",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewState(tt.cluster)
			if tt.effects != nil {
				tt.effects(s)
			}
			events, cluster := s.Apply()
			if !reflect.DeepEqual(events, tt.wantEvents) {
				t.Errorf("State.Apply() events = %v, wantEvents %v", events, tt.wantEvents)
			}
			var actual *v1alpha1.ElasticsearchStatus
			if cluster != nil {
				actual = &cluster.Status
			}
			if !reflect.DeepEqual(actual, tt.wantStatus) {
				t.Errorf("State.Apply() cluster = %v, wantStatus %v", cluster.Status, tt.wantStatus)
			}
		})
	}
}

func TestState_UpdateElasticsearchState(t *testing.T) {
	type args struct {
		resourcesState ResourcesState
		observedState  observer.State
	}
	tests := []struct {
		name            string
		cluster         v1alpha1.Elasticsearch
		args            args
		stateAssertions func(s *State)
	}{
		{
			name: "phase is not changed by default",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Phase: v1alpha1.ElasticsearchApplyingChangesPhase,
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, v1alpha1.ElasticsearchApplyingChangesPhase, s.status.Phase)
			},
		},
		{
			name:    "health is unknown by default",
			cluster: v1alpha1.Elasticsearch{},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "unknown", s.status.Health)
			},
		},
		{
			name:    "health is set if returned by Elasticsearch",
			cluster: v1alpha1.Elasticsearch{},
			args: args{
				observedState: observer.State{
					ClusterHealth: &client.Health{Status: "green"},
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "green", s.status.Health)

			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewState(tt.cluster)
			s.UpdateElasticsearchState(tt.args.resourcesState, tt.args.observedState)
			if tt.stateAssertions != nil {
				tt.stateAssertions(s)
			}
		})
	}
}

func TestState_UpdateElasticsearchMigrating(t *testing.T) {
	type args struct {
		result         reconcile.Result
		resourcesState ResourcesState
		observedState  observer.State
	}
	tests := []struct {
		name            string
		cluster         v1alpha1.Elasticsearch
		args            args
		stateAssertions func(s *State)
	}{
		{
			name:    "base case",
			cluster: v1alpha1.Elasticsearch{},
			args: args{
				result: reconcile.Result{RequeueAfter: 10 * time.Minute},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, v1alpha1.ElasticsearchMigratingDataPhase, s.status.Phase)
				assert.Equal(t, []events.Event{{EventType: corev1.EventTypeNormal, Reason: events.EventReasonDelayed, Message: "Requested topology change delayed by data migration"}}, s.Recorder.Events())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewState(tt.cluster)
			s.UpdateElasticsearchMigrating(tt.args.resourcesState, tt.args.observedState)
			if tt.stateAssertions != nil {
				tt.stateAssertions(s)
			}
		})
	}
}
