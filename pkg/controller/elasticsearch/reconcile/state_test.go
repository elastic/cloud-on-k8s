// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconcile

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
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
		cluster    esv1.Elasticsearch
		effects    func(s *State)
		wantEvents []events.Event
		wantStatus *esv1.ElasticsearchStatus
	}{
		{
			name:       "defaults",
			cluster:    esv1.Elasticsearch{},
			wantEvents: []events.Event{},
			wantStatus: &esv1.ElasticsearchStatus{
				AvailableNodes: 0,
				Health:         esv1.ElasticsearchUnknownHealth,
				Phase:          "",
			},
		},
		{
			name:    "no degraded health event on cluster formation",
			cluster: esv1.Elasticsearch{},
			effects: func(s *State) {
				s.UpdateWithPhase(esv1.ElasticsearchApplyingChangesPhase)
			},
			wantEvents: []events.Event{},
			wantStatus: &esv1.ElasticsearchStatus{
				AvailableNodes: 0,
				Health:         esv1.ElasticsearchUnknownHealth,
				Phase:          esv1.ElasticsearchApplyingChangesPhase,
			},
		},
		{
			name: "no degraded health event when cluster info is unknown",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Health: esv1.ElasticsearchGreenHealth,
				},
			},
			effects: func(s *State) {
				s.UpdateClusterHealth(esv1.ElasticsearchUnknownHealth)
			},
			wantEvents: []events.Event{},
			wantStatus: &esv1.ElasticsearchStatus{
				AvailableNodes: 0,
				Health:         esv1.ElasticsearchUnknownHealth,
				Phase:          "",
			},
		},
		{
			name: "health degraded",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Health: esv1.ElasticsearchGreenHealth,
				},
			},
			effects: func(s *State) {
				s.UpdateWithPhase(esv1.ElasticsearchApplyingChangesPhase).UpdateClusterHealth(esv1.ElasticsearchRedHealth)
			},
			wantEvents: []events.Event{{EventType: corev1.EventTypeWarning, Reason: events.EventReasonUnhealthy, Message: "Elasticsearch cluster health degraded"}},
			wantStatus: &esv1.ElasticsearchStatus{
				AvailableNodes: 0,
				Health:         esv1.ElasticsearchRedHealth,
				Phase:          esv1.ElasticsearchApplyingChangesPhase,
			},
		},
		{
			name: "Status.observedGeneration is set from metadata.generation",
			cluster: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Generation: int64(1),
				},
			},
			effects: func(s *State) {
				s.UpdateWithPhase(esv1.ElasticsearchApplyingChangesPhase).UpdateClusterHealth(esv1.ElasticsearchRedHealth)
			},
			wantEvents: []events.Event{},
			wantStatus: &esv1.ElasticsearchStatus{
				ObservedGeneration: int64(1),
				AvailableNodes:     0,
				Health:             esv1.ElasticsearchRedHealth,
				Phase:              esv1.ElasticsearchApplyingChangesPhase,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := MustNewState(tt.cluster)
			if tt.effects != nil {
				tt.effects(s)
			}
			events, cluster := s.Apply()
			if !reflect.DeepEqual(events, tt.wantEvents) {
				t.Errorf("State.Apply() events = %v, wantEvents %v", events, tt.wantEvents)
			}
			var actual *esv1.ElasticsearchStatus
			if cluster != nil {
				actual = &cluster.Status
			}
			assertSemanticEqualStatuses(t, actual, tt.wantStatus)
		})
	}
}

func assertSemanticEqualStatuses(t *testing.T, actual, expected *esv1.ElasticsearchStatus) {
	t.Helper()
	if expected == nil || actual == nil {
		if !(expected == actual) {
			t.Errorf("State.Apply() cluster = %v, wantStatus %v", actual, expected)
			return
		}
	}
	assert.EqualValues(t, expected.Version, actual.Version)
	assert.EqualValues(t, expected.Phase, actual.Phase)
	assert.EqualValues(t, expected.Health, actual.Health)
	assert.EqualValues(t, expected.Health, actual.Health)
	assert.ElementsMatch(t, expected.DownscaleOperation.Nodes, actual.DownscaleOperation.Nodes)
	assert.EqualValues(t, expected.DownscaleOperation.Stalled, actual.DownscaleOperation.Stalled)
	assert.ElementsMatch(t, expected.UpscaleOperation.Nodes, actual.UpscaleOperation.Nodes)
	assert.ElementsMatch(t, expected.UpgradeOperation.Nodes, actual.UpgradeOperation.Nodes)
}

func TestState_UpdateElasticsearchState(t *testing.T) {
	type args struct {
		resourcesState ResourcesState
		observedHealth esv1.ElasticsearchHealth
	}
	tests := []struct {
		name            string
		cluster         esv1.Elasticsearch
		args            args
		stateAssertions func(s *State)
	}{
		{
			name: "phase is defaulting to empty",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Phase: esv1.ElasticsearchApplyingChangesPhase,
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "", s.status.Phase)
			},
		},
		{
			name: "version is not changed by default",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Phase:   esv1.ElasticsearchApplyingChangesPhase,
					Version: "7.7.0",
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "7.7.0", s.status.Version)
			},
		},
		{
			name: "version is changed if necessary",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Phase:   esv1.ElasticsearchApplyingChangesPhase,
					Version: "7.7.0",
				},
			},
			args: args{
				resourcesState: ResourcesState{AllPods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{label.VersionLabelName: "7.8.0"}}}},
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "7.8.0", s.status.Version)
			},
		},
		{
			name: "version is not changed if it cannot be parsed",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Phase:   esv1.ElasticsearchApplyingChangesPhase,
					Version: "7.7.0",
				},
			},
			args: args{
				resourcesState: ResourcesState{AllPods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{label.VersionLabelName: "invalid"}}}},
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "7.7.0", s.status.Version)
			},
		},
		{
			name:    "health is unknown by default",
			cluster: esv1.Elasticsearch{},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, esv1.ElasticsearchUnknownHealth, s.status.Health)
			},
		},
		{
			name:    "health is set if returned by Elasticsearch",
			cluster: esv1.Elasticsearch{},
			args: args{
				observedHealth: esv1.ElasticsearchGreenHealth,
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, "green", s.status.Health)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := MustNewState(tt.cluster)
			s.UpdateClusterHealth(tt.args.observedHealth).UpdateMinRunningVersion(tt.args.resourcesState)
			if tt.stateAssertions != nil {
				tt.stateAssertions(s)
			}
		})
	}
}

func TestState_UpdateMinRunningVersion(t *testing.T) {
	ssetWithVersion := func(value string) appsv1.StatefulSet {
		return appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{label.VersionLabelName: value}}}}}
	}
	podWithVersion := func(value string) corev1.Pod {
		return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{label.VersionLabelName: value}}}
	}
	type want struct {
		ver       string         // expected version to be set in the status
		condition esv1.Condition // expected esv1.RunningDesiredVersion condition
	}
	tests := []struct {
		name           string
		resourcesState ResourcesState
		es             esv1.Elasticsearch
		want           want
	}{
		{
			name: "all pods and ssets specify the same version",
			es:   esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "7.7.0"}},
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.0"), podWithVersion("7.7.0")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.0"), ssetWithVersion("7.7.0")},
			},
			want: want{
				ver: "7.7.0",
				condition: esv1.Condition{
					Type:    esv1.RunningDesiredVersion,
					Status:  corev1.ConditionTrue,
					Message: "All nodes are running version 7.7.0",
				},
			},
		},
		{
			name: "one Pod has not been upgraded yet",
			es:   esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "7.7.1"}},
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.1"), podWithVersion("7.7.0")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.1"), ssetWithVersion("7.7.1")},
			},
			want: want{
				ver: "7.7.0",
				condition: esv1.Condition{
					Type:    esv1.RunningDesiredVersion,
					Status:  corev1.ConditionFalse,
					Message: "Upgrading from 7.7.0 to 7.7.1",
				},
			},
		},
		{
			name: "one StatefulSet (whose Pods are missing) has not been upgraded yet",
			es:   esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "7.7.1"}},
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.1")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.1"), ssetWithVersion("7.7.0")},
			},
			want: want{
				ver: "7.7.0",
				condition: esv1.Condition{
					Type:    esv1.RunningDesiredVersion,
					Status:  corev1.ConditionFalse,
					Message: "Upgrading from 7.7.0 to 7.7.1",
				},
			},
		},
		{
			name: "invalid version in the labels",
			es:   esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "7.7.0"}},
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.1")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("invalid"), ssetWithVersion("7.7.0")},
			},
			want: want{
				ver: "",
				condition: esv1.Condition{
					Type:    esv1.RunningDesiredVersion,
					Status:  corev1.ConditionUnknown,
					Message: "No running version reported",
				},
			},
		},
		{
			name: "invalid version in spec",
			es:   esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "invalid"}},
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.0")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.0"), ssetWithVersion("7.7.0")},
			},
			want: want{
				ver: "7.7.0",
				condition: esv1.Condition{
					Type:    esv1.RunningDesiredVersion,
					Status:  corev1.ConditionUnknown,
					Message: "Error while parsing desired version: No Major.Minor.Patch elements found",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewState(tt.es)
			assert.NoError(t, err)
			got := s.UpdateMinRunningVersion(tt.resourcesState)
			conditionIndex := got.Conditions.Index(esv1.RunningDesiredVersion)
			if conditionIndex < 0 {
				t.Fatalf("Elasticsearch status should contain condition esv1.RunningDesiredVersion")
			}
			condition := got.Conditions[conditionIndex]
			if !conditionsEqual(condition, tt.want.condition) {
				t.Errorf("fetchMinRunningVersion() status.Condition[esv1.RunningDesiredVersion] = %v, want %v", condition, tt.want.condition)
			}
			if !reflect.DeepEqual(got.status.Version, tt.want.ver) {
				t.Errorf("fetchMinRunningVersion() status.Version = %v, want %v", got.status.Version, tt.want.ver)
			}
		})
	}
}

func conditionsEqual(c1, c2 esv1.Condition) bool {
	return c1.Message == c2.Message &&
		c1.Type == c2.Type &&
		c1.Status == c2.Status
}

func TestState_UpdateWithPhase(t *testing.T) {
	tests := []struct {
		name   string
		status esv1.ElasticsearchStatus
		phase  esv1.ElasticsearchOrchestrationPhase
		want   esv1.ElasticsearchOrchestrationPhase
	}{
		{
			name:   "empty default can always be overridden",
			status: esv1.ElasticsearchStatus{},
			phase:  esv1.ElasticsearchApplyingChangesPhase,
			want:   esv1.ElasticsearchApplyingChangesPhase,
		},
		{
			name: "Invalid phase is sticky",
			status: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchResourceInvalid,
			},
			phase: esv1.ElasticsearchReadyPhase,
			want:  esv1.ElasticsearchResourceInvalid,
		},
		{
			name: "ApplyingChanges must not override non-ready phases",
			status: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchMigratingDataPhase,
			},
			phase: esv1.ElasticsearchApplyingChangesPhase,
			want:  esv1.ElasticsearchMigratingDataPhase,
		},
		{
			name:   "ApplyingChanges can be set if no other phase is set",
			status: esv1.ElasticsearchStatus{},
			phase:  esv1.ElasticsearchApplyingChangesPhase,
			want:   esv1.ElasticsearchApplyingChangesPhase,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{
				status: tt.status,
			}
			assert.Equalf(t, tt.want, s.UpdateWithPhase(tt.phase).status.Phase, "UpdateWithPhase(%v)", tt.phase)
		})
	}
}
