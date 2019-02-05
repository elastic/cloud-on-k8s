package mutation

import (
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/pod"
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
						ToCreate: []PodToCreate{{Pod: namedPod("1")}},
						ToDelete: []corev1.Pod{namedPod("2")},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{"2": namedPod("2")},
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
						{Pod: namedPod("1"), PodSpecCtx: pod.PodSpecContext{}},
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
						ToCreate: []PodToCreate{{Pod: namedPod("1")}},
						ToDelete: []corev1.Pod{namedPod("2")},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{"2": namedPod("2"), "3": namedPod("3")},
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
					ToDelete: []corev1.Pod{
						namedPod("2"),
					},
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
						ToCreate: []PodToCreate{{Pod: namedPod("create-1")}, {Pod: namedPod("create-2")}},
						ToKeep:   []corev1.Pod{namedPod("keep-3")},
						ToDelete: []corev1.Pod{namedPod("delete-1"), namedPod("delete-2")},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{
							"delete-1": namedPod("delete-1"),
							"delete-2": namedPod("delete-2"),
							"keep-3":   namedPod("keep-3"),
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
						{Pod: namedPod("create-1"), PodSpecCtx: pod.PodSpecContext{}},
					},
					ToDelete: []corev1.Pod{
						namedPod("delete-1"),
					},
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
					ToCreate: []PodToCreate{{Pod: namedPod("create-1")}, {Pod: namedPod("create-2")}},
					ToKeep:   []corev1.Pod{namedPod("keep-3")},
					ToDelete: []corev1.Pod{namedPod("delete-1"), namedPod("delete-2")},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{
						"delete-1": namedPod("delete-1"),
						"delete-2": namedPod("delete-2"),
						"keep-3":   namedPod("keep-3"),
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
					ToKeep:   []corev1.Pod{namedPod("bar")},
					ToDelete: []corev1.Pod{namedPod("foo"), namedPod("baz")},
				},
				PodsState: initializePodsState(PodsState{
					Deleting:     map[string]corev1.Pod{"baz": namedPod("baz")},
					RunningReady: map[string]corev1.Pod{"foo": namedPod("foo"), "bar": namedPod("bar")},
				}),
			},
			args: args{
				performableChanges: PerformableChanges{
					Changes: Changes{
						ToDelete: []corev1.Pod{namedPod("foo")},
					},
				},
			},
			want: ChangeGroup{
				Changes: Changes{
					ToKeep:   []corev1.Pod{namedPod("bar")},
					ToDelete: []corev1.Pod{namedPod("baz")},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{"bar": namedPod("bar")},
					Deleting:     map[string]corev1.Pod{"foo": namedPod("foo"), "baz": namedPod("baz")},
				}),
			},
		},
		{
			name: "creation",
			fields: fields{
				Changes: Changes{
					ToKeep:   []corev1.Pod{namedPod("bar")},
					ToCreate: []PodToCreate{{Pod: namedPod("foo")}, {Pod: namedPod("baz")}},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{"bar": namedPod("bar")},
				}),
			},
			args: args{
				performableChanges: PerformableChanges{
					Changes: Changes{
						ToCreate: []PodToCreate{{Pod: namedPod("foo")}},
					},
				},
			},
			want: ChangeGroup{
				Changes: Changes{
					ToCreate: []PodToCreate{{Pod: namedPod("baz")}},
					ToKeep:   []corev1.Pod{namedPod("bar"), namedPod("foo")},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{"bar": namedPod("bar")},
					Pending:      map[string]corev1.Pod{"foo": namedPod("foo")},
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
