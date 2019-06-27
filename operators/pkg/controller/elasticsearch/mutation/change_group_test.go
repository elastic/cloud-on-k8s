// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestChangeGroups_CalculatePerformableChanges(t *testing.T) {
	tests := []struct {
		name               string
		s                  ChangeGroups
		budget             v1alpha1.ChangeBudget
		podRestrictions    PodRestrictions
		performableChanges *PerformableChanges
		want               *PerformableChanges
		wantErr            bool
	}{
		{
			name:               "empty",
			s:                  ChangeGroups{},
			performableChanges: &PerformableChanges{},
			want:               &PerformableChanges{},
		},
		{
			name: "can only create if unavailable budget is maxed out",
			s: ChangeGroups{
				ChangeGroup{
					Name: "foo",
					Changes: Changes{
						ToCreate: []PodToCreate{{Pod: namedPod("1").Pod}},
						ToDelete: PodsToDelete{{PodWithConfig: namedPod("2")}},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{"2": namedPod("2").Pod},
					}),
				},
			},
			performableChanges: &PerformableChanges{},
			budget: v1alpha1.ChangeBudget{
				MaxSurge:       1,
				MaxUnavailable: 0,
			},
			want: &PerformableChanges{
				Changes: Changes{
					ToCreate: []PodToCreate{
						{Pod: namedPod("1").Pod, PodSpecCtx: pod.PodSpecContext{}},
					},
				},
				MaxUnavailableGroups: []string{"foo"},
			},
		},
		{
			name: "can only delete if surge budget is maxed out",
			s: ChangeGroups{
				ChangeGroup{
					Name: "foo",
					Changes: Changes{
						ToCreate: []PodToCreate{{Pod: namedPod("1").Pod}},
						ToDelete: PodsToDelete{{PodWithConfig: namedPod("2")}},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{"2": namedPod("2").Pod, "3": namedPod("3").Pod},
					}),
				},
			},
			performableChanges: &PerformableChanges{},
			budget: v1alpha1.ChangeBudget{
				MaxSurge:       1,
				MaxUnavailable: 1,
			},
			want: &PerformableChanges{
				Changes: Changes{
					ToDelete: PodsToDelete{{PodWithConfig: namedPod("2")}},
				},
				MaxSurgeGroups: []string{"foo"},
			},
		},
		{
			name: "can both delete and create up to the surge and unavailability budgets are exhausted",
			s: ChangeGroups{
				ChangeGroup{
					Name: "foo",
					Changes: Changes{
						ToCreate: []PodToCreate{{Pod: namedPod("create-1").Pod}, {Pod: namedPod("create-2").Pod}},
						ToKeep:   pod.PodsWithConfig{namedPod("keep-3")},
						ToDelete: PodsToDelete{{PodWithConfig: namedPod("delete-1")}, {PodWithConfig: namedPod("delete-2")}},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{
							"delete-1": namedPod("delete-1").Pod,
							"delete-2": namedPod("delete-2").Pod,
							"keep-3":   namedPod("keep-3").Pod,
						},
					}),
				},
			},
			performableChanges: &PerformableChanges{},
			budget: v1alpha1.ChangeBudget{
				MaxSurge:       1,
				MaxUnavailable: 1,
			},
			want: &PerformableChanges{
				Changes: Changes{
					ToCreate: []PodToCreate{
						{Pod: namedPod("create-1").Pod, PodSpecCtx: pod.PodSpecContext{}},
					},
					ToDelete: PodsToDelete{{PodWithConfig: namedPod("delete-1")}},
				},
				MaxSurgeGroups:       []string{"foo"},
				MaxUnavailableGroups: []string{"foo"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.calculatePerformableChanges(tt.budget, &tt.podRestrictions, tt.performableChanges)
			if (err != nil) != tt.wantErr {
				t.Errorf("ChangeGroups.calculatePerformableChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, tt.performableChanges)
		})
	}
}

func TestChangeGroups_ChangeStats(t *testing.T) {
	type fields struct {
		Name       string
		Definition v1alpha1.GroupingDefinition
		Changes    Changes
		PodsState  PodsState
	}
	tests := []struct {
		name   string
		fields fields
		want   ChangeStats
	}{
		{
			name: "sample",
			fields: fields{
				Definition: v1alpha1.GroupingDefinition{
					Selector: metav1.LabelSelector{},
				},
				Changes: Changes{
					ToCreate: []PodToCreate{{Pod: namedPod("create-1").Pod}, {Pod: namedPod("create-2").Pod}},
					ToKeep:   pod.PodsWithConfig{namedPod("keep-3")},
					ToDelete: PodsToDelete{{PodWithConfig: namedPod("delete-1")}, {PodWithConfig: namedPod("delete-2")}},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{
						"delete-1": namedPod("delete-1").Pod,
						"delete-2": namedPod("delete-2").Pod,
						"keep-3":   namedPod("keep-3").Pod,
					},
				}),
			},
			want: ChangeStats{
				TargetPods:              3,
				CurrentPods:             3,
				CurrentSurge:            0,
				CurrentRunningReadyPods: 3,
				CurrentUnavailable:      0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ChangeGroup{
				Name:      tt.fields.Name,
				Changes:   tt.fields.Changes,
				PodsState: tt.fields.PodsState,
			}

			assert.Equal(t, tt.want, s.ChangeStats())
		})
	}
}

func TestChangeGroups_simulatePerformableChangesApplied(t *testing.T) {
	type fields struct {
		Name      string
		Changes   Changes
		PodsState PodsState
	}
	type args struct {
		performableChanges PerformableChanges
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   ChangeGroup
	}{
		{
			name: "deletion",
			fields: fields{
				Changes: Changes{
					ToKeep:   pod.PodsWithConfig{namedPod("bar")},
					ToDelete: PodsToDelete{{PodWithConfig: namedPod("foo")}, {PodWithConfig: namedPod("baz")}},
				},
				PodsState: initializePodsState(PodsState{
					Deleting:     map[string]corev1.Pod{"baz": namedPod("baz").Pod},
					RunningReady: map[string]corev1.Pod{"foo": namedPod("foo").Pod, "bar": namedPod("bar").Pod},
				}),
			},
			args: args{
				performableChanges: PerformableChanges{
					Changes: Changes{
						ToDelete: PodsToDelete{{PodWithConfig: namedPod("foo")}},
					},
				},
			},
			want: ChangeGroup{
				Changes: Changes{
					ToKeep:   pod.PodsWithConfig{namedPod("bar")},
					ToDelete: PodsToDelete{{PodWithConfig: namedPod("baz")}},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{"bar": namedPod("bar").Pod},
					Deleting:     map[string]corev1.Pod{"foo": namedPod("foo").Pod, "baz": namedPod("baz").Pod},
				}),
			},
		},
		{
			name: "creation",
			fields: fields{
				Changes: Changes{
					ToKeep:   pod.PodsWithConfig{namedPod("bar")},
					ToCreate: []PodToCreate{{Pod: namedPod("foo").Pod}, {Pod: namedPod("baz").Pod}},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{"bar": namedPod("bar").Pod},
				}),
			},
			args: args{
				performableChanges: PerformableChanges{
					Changes: Changes{
						ToCreate: []PodToCreate{{Pod: namedPod("foo").Pod}},
					},
				},
			},
			want: ChangeGroup{
				Changes: Changes{
					ToCreate: []PodToCreate{{Pod: namedPod("baz").Pod}},
					ToKeep:   pod.PodsWithConfig{namedPod("bar"), namedPod("foo")},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{"bar": namedPod("bar").Pod},
					Pending:      map[string]corev1.Pod{"foo": namedPod("foo").Pod},
				}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ChangeGroup{
				Name:      tt.fields.Name,
				Changes:   tt.fields.Changes,
				PodsState: tt.fields.PodsState,
			}
			s.simulatePerformableChangesApplied(tt.args.performableChanges)

			assert.Equal(t, &tt.want, s)
		})
	}
}
