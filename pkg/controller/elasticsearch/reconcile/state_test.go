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
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
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
			wantStatus: &esv1.ElasticsearchStatus{},
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
	assert.ElementsMatch(t, expected.RollingUpgradeOperation.Nodes, actual.RollingUpgradeOperation.Nodes)
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
			name: "phase is not changed by default",
			cluster: esv1.Elasticsearch{
				Status: esv1.ElasticsearchStatus{
					Phase: esv1.ElasticsearchApplyingChangesPhase,
				},
			},
			stateAssertions: func(s *State) {
				assert.EqualValues(t, esv1.ElasticsearchApplyingChangesPhase, s.status.Phase)
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

func TestState_fetchMinRunningVersion(t *testing.T) {
	v770 := version.MustParse("7.7.0")
	ssetWithVersion := func(value string) appsv1.StatefulSet {
		return appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{label.VersionLabelName: value}}}}}
	}
	podWithVersion := func(value string) corev1.Pod {
		return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{label.VersionLabelName: value}}}
	}
	tests := []struct {
		name           string
		resourcesState ResourcesState
		want           *version.Version
		wantErr        bool
	}{
		{
			name: "all pods and ssets specify the same version",
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.0"), podWithVersion("7.7.0")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.0"), ssetWithVersion("7.7.0")},
			},
			want: &v770,
		},
		{
			name: "one Pod has not been upgraded yet",
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.1"), podWithVersion("7.7.0")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.1"), ssetWithVersion("7.7.1")},
			},
			want: &v770,
		},
		{
			name: "one StatefulSet (whose Pods are missing) has not been upgraded yet",
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.1")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("7.7.1"), ssetWithVersion("7.7.0")},
			},
			want: &v770,
		},
		{
			name: "invalid version in the labels: error out",
			resourcesState: ResourcesState{
				AllPods:      []corev1.Pod{podWithVersion("7.7.1")},
				StatefulSets: []appsv1.StatefulSet{ssetWithVersion("invalid"), ssetWithVersion("7.7.0")},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{}
			got, err := s.fetchMinRunningVersion(tt.resourcesState)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchMinRunningVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchMinRunningVersion() got = %v, want %v", got, tt.want)
			}
		})
	}
}
