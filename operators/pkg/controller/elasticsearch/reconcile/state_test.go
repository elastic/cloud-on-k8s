// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"reflect"
	"testing"
	"time"

	v1alpha12 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/observer"
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
				s.UpdateElasticsearchPending([]corev1.Pod{})
			},
			wantEvents: []events.Event{},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health: v1alpha1.ElasticsearchRedHealth,
				Phase:  v1alpha1.ElasticsearchPendingPhase,
			},
		},
		{
			name: "no degraded health event when cluster state is unknown",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health: v1alpha1.ElasticsearchGreenHealth,
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchState(ResourcesState{}, observer.State{
					ClusterHealth: nil,
					ClusterState:  nil,
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
				s.UpdateElasticsearchPending([]corev1.Pod{})
			},
			wantEvents: []events.Event{{corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elasticsearch cluster health degraded"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health: v1alpha1.ElasticsearchRedHealth,
				Phase:  v1alpha1.ElasticsearchPendingPhase,
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
				s.UpdateElasticsearchOperational(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						ClusterUUID: "new",
					},
				})
			},
			wantEvents: []events.Event{{corev1.EventTypeWarning, events.EventReasonUnexpected, "Cluster UUID changed (was: old, is: new)"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:      v1alpha1.ElasticsearchRedHealth,
				Phase:       v1alpha1.ElasticsearchOperationalPhase,
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
				s.UpdateElasticsearchOperational(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
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
				Phase:       v1alpha1.ElasticsearchOperationalPhase,
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
				s.UpdateElasticsearchOperational(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
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
				Phase:       v1alpha1.ElasticsearchOperationalPhase,
				ClusterUUID: "old",
			},
		},
		{
			name: "master node changed",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health:     v1alpha1.ElasticsearchRedHealth,
					MasterNode: "old",
					Phase:      v1alpha1.ElasticsearchOperationalPhase,
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchState(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						MasterNode: "new",
						Nodes: map[string]client.ClusterStateNode{
							"new": {Name: "new"},
						},
					},
				})
			},
			wantEvents: []events.Event{{corev1.EventTypeNormal, events.EventReasonStateChange, "Master node is now new"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:     v1alpha1.ElasticsearchRedHealth,
				Phase:      v1alpha1.ElasticsearchOperationalPhase,
				MasterNode: "new",
			},
		},
		{
			name: "ignore temporary master loss for status",
			cluster: v1alpha1.Elasticsearch{
				Status: v1alpha1.ElasticsearchStatus{
					Health:     v1alpha1.ElasticsearchRedHealth,
					MasterNode: "old",
					Phase:      v1alpha1.ElasticsearchOperationalPhase,
				},
			},
			effects: func(s *State) {
				s.UpdateElasticsearchState(ResourcesState{}, observer.State{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						MasterNode: "",
					},
				})
			},
			wantEvents: []events.Event{},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:     v1alpha1.ElasticsearchRedHealth,
				Phase:      v1alpha1.ElasticsearchOperationalPhase,
				MasterNode: "",
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
					Phase: v1alpha1.ElasticsearchPendingPhase,
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, v1alpha1.ElasticsearchPendingPhase, s.status.Phase)
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
				assert.Equal(t, []events.Event{{corev1.EventTypeNormal, events.EventReasonDelayed, "Requested topology change delayed by data migration"}}, s.Recorder.Events())
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
