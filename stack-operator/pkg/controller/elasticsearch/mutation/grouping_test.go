package mutation

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestGroupedChangeSets_CalculatePerformableChanges(t *testing.T) {
	tests := []struct {
		name               string
		s                  GroupedChangeSets
		budget             v1alpha1.ChangeBudget
		performableChanges *PerformableChanges
		want               *PerformableChanges
		wantErr            bool
	}{
		{
			name:               "empty",
			s:                  GroupedChangeSets{},
			performableChanges: &PerformableChanges{},
			want:               &PerformableChanges{},
		},
		{
			name: "can only add if unavailable budget is maxed out",
			s: GroupedChangeSets{
				GroupedChangeSet{
					Name: "foo",
					ChangeSet: ChangeSet{
						ToAdd: []corev1.Pod{namedPod("1")},
						ToAddContext: map[string]support.PodToAdd{
							"1": {},
						},
						ToRemove: []corev1.Pod{namedPod("2")},
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
				ScheduleForCreation: []CreatablePod{
					{Pod: namedPod("1"), PodSpecContext: support.PodSpecContext{}},
				},
				MaxUnavailableGroups: []string{"foo"},
			},
		},
		{
			name: "can only remove if surge budget is maxed out",
			s: GroupedChangeSets{
				GroupedChangeSet{
					Name: "foo",
					ChangeSet: ChangeSet{
						ToAdd: []corev1.Pod{namedPod("1")},
						ToAddContext: map[string]support.PodToAdd{
							"1": {},
						},
						ToRemove: []corev1.Pod{namedPod("2")},
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
				ScheduleForDeletion: []corev1.Pod{
					namedPod("2"),
				},
				MaxSurgeGroups: []string{"foo"},
			},
		},
		{
			name: "can both remove and add up to the surge and unavailability budgets are exhausted",
			s: GroupedChangeSets{
				GroupedChangeSet{
					Name: "foo",
					ChangeSet: ChangeSet{
						ToAdd: []corev1.Pod{namedPod("add-1"), namedPod("add-2")},
						ToAddContext: map[string]support.PodToAdd{
							"1": {},
						},
						ToKeep:   []corev1.Pod{namedPod("keep-3")},
						ToRemove: []corev1.Pod{namedPod("remove-1"), namedPod("remove-2")},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{
							"remove-1": namedPod("remove-1"),
							"remove-2": namedPod("remove-2"),
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
				ScheduleForCreation: []CreatablePod{
					{Pod: namedPod("add-1"), PodSpecContext: support.PodSpecContext{}},
				},
				ScheduleForDeletion: []corev1.Pod{
					namedPod("remove-1"),
				},
				MaxSurgeGroups:       []string{"foo"},
				MaxUnavailableGroups: []string{"foo"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.CalculatePerformableChanges(tt.budget, tt.performableChanges)
			if (err != nil) != tt.wantErr {
				t.Errorf("GroupedChangeSets.CalculatePerformableChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, tt.performableChanges)
		})
	}
}

func TestGroupedChangeSet_KeyNumbers(t *testing.T) {
	type fields struct {
		Name       string
		Definition v1alpha1.GroupingDefinition
		ChangeSet  ChangeSet
		PodsState  PodsState
	}
	tests := []struct {
		name   string
		fields fields
		want   KeyNumbers
	}{
		{
			name: "sample",
			fields: fields{
				Definition: v1alpha1.GroupingDefinition{
					Selector: v1.LabelSelector{},
				},
				ChangeSet: ChangeSet{
					ToAdd: []corev1.Pod{namedPod("add-1"), namedPod("add-2")},
					ToAddContext: map[string]support.PodToAdd{
						"1": {},
					},
					ToKeep:   []corev1.Pod{namedPod("keep-3")},
					ToRemove: []corev1.Pod{namedPod("remove-1"), namedPod("remove-2")},
				},
				PodsState: initializePodsState(PodsState{
					RunningReady: map[string]corev1.Pod{
						"remove-1": namedPod("remove-1"),
						"remove-2": namedPod("remove-2"),
						"keep-3":   namedPod("keep-3"),
					},
				}),
			},
			want: KeyNumbers{
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
			s := GroupedChangeSet{
				Name:      tt.fields.Name,
				ChangeSet: tt.fields.ChangeSet,
				PodsState: tt.fields.PodsState,
			}

			assert.Equal(t, tt.want, s.KeyNumbers())
		})
	}
}
