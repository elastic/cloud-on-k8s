package support

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func namedPodWithCreationTimestamp(name string, creationTimestamp time.Time) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:              name,
			CreationTimestamp: v1.Time{Time: creationTimestamp},
		},
	}
}

func withLabels(pod corev1.Pod, labels map[string]string) corev1.Pod {
	pod.Labels = labels
	return pod
}

func TestChangeSet_IsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		changeSet ChangeSet
		want      bool
	}{
		{
			name:      "empty",
			changeSet: ChangeSet{},
			want:      true,
		},
		{
			name: "toAdd",
			changeSet: ChangeSet{
				ToAdd: []corev1.Pod{{}},
			},
			want: false,
		},
		{
			name: "toKeep",
			changeSet: ChangeSet{
				ToKeep: []corev1.Pod{{}},
			},
			want: false,
		},
		{
			name: "toRemove",
			changeSet: ChangeSet{
				ToRemove: []corev1.Pod{{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.changeSet
			if got := s.IsEmpty(); got != tt.want {
				t.Errorf("ChangeSet.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewChangeSetFromChanges(t *testing.T) {
	examplePodToAdd := PodToAdd{PodSpecCtx: PodSpecContext{PodSpec: corev1.PodSpec{Hostname: "foo"}}}
	type args struct {
		changes    Changes
		newPodFunc NewPodFunc
	}
	tests := []struct {
		name    string
		args    args
		want    *ChangeSet
		wantErr bool
	}{
		{
			name: "empty",
			args: args{},
			want: &ChangeSet{
				ToAdd:        []corev1.Pod{},
				ToAddContext: map[string]PodToAdd{},
			},
			wantErr: false,
		},
		{
			name: "with pods",
			args: args{
				changes: Changes{
					ToAdd: []PodToAdd{
						examplePodToAdd,
					},
					ToKeep:   []corev1.Pod{namedPod("2")},
					ToRemove: []corev1.Pod{namedPod("3")},
				},
				newPodFunc: func(ctx PodSpecContext) (corev1.Pod, error) {
					if !reflect.DeepEqual(ctx, examplePodToAdd.PodSpecCtx) {
						return corev1.Pod{}, fmt.Errorf("got unexpected parameter: %v", ctx)
					}
					return namedPod("example"), nil
				},
			},
			want: &ChangeSet{
				ToAdd:        []corev1.Pod{namedPod("example")},
				ToAddContext: map[string]PodToAdd{"example": examplePodToAdd},

				ToKeep:   []corev1.Pod{namedPod("2")},
				ToRemove: []corev1.Pod{namedPod("3")},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewChangeSetFromChanges(tt.args.changes, tt.args.newPodFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewChangeSetFromChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGroupedChangeSets_ValidateMasterChanges(t *testing.T) {
	masterNode := namedPodWithCreationTimestamp("master", time.Unix(5, 0))
	masterNode.Labels = NodeTypesMasterLabelName.AsMap(true)

	tests := []struct {
		name    string
		s       GroupedChangeSets
		wantErr bool
	}{
		{
			name:    "no groups",
			s:       GroupedChangeSets{},
			wantErr: false,
		},
		{
			name: "one group no masters",
			s: GroupedChangeSets{
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{namedPod("1")}}},
			},
			wantErr: false,
		},
		{
			name: "two groups no masters",
			s: GroupedChangeSets{
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{namedPod("1")}}},
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{namedPod("1")}}},
			},
			wantErr: false,
		},
		{
			name: "one group one master",
			s: GroupedChangeSets{
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{masterNode}}},
			},
			wantErr: false,
		},
		{
			name: "two groups one master",
			s: GroupedChangeSets{
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{masterNode}}},
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{namedPod("1")}}},
			},
			wantErr: false,
		},
		{
			name: "two groups two masters",
			s: GroupedChangeSets{
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{masterNode}}},
				GroupedChangeSet{ChangeSet: ChangeSet{ToRemove: []corev1.Pod{masterNode}}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.s.ValidateMasterChanges(); (err != nil) != tt.wantErr {
				t.Errorf("GroupedChangeSets.ValidateMasterChanges() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChangeSet_Group(t *testing.T) {
	fooMatchingGroupingDefinition := v1alpha1.GroupingDefinition{
		Selector: v1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
	}

	fooPod := withLabels(namedPod("1"), map[string]string{"foo": "bar"})

	barPod := withLabels(namedPod("2"), map[string]string{"bar": "bar"})
	bazPod := withLabels(namedPod("3"), map[string]string{"baz": "bar"})
	bazPodToAdd := PodToAdd{
		PodSpecCtx: PodSpecContext{PodSpec: corev1.PodSpec{Hostname: "baz"}},
	}
	emptyChangeSet := ChangeSet{
		ToAdd:        make([]corev1.Pod, 0),
		ToRemove:     make([]corev1.Pod, 0),
		ToKeep:       make([]corev1.Pod, 0),
		ToAddContext: make(map[string]PodToAdd),
	}

	type args struct {
		groupingDefinitions []v1alpha1.GroupingDefinition
		remainingPodsState  PodsState
	}
	tests := []struct {
		name      string
		changeSet ChangeSet
		args      args
		want      GroupedChangeSets
		wantErr   bool
	}{
		{
			name:      "empty",
			changeSet: ChangeSet{},
			args:      args{},
			want:      GroupedChangeSets{},
		},
		{
			name:      "no group definitions should result in defaulted group",
			changeSet: ChangeSet{ToKeep: []corev1.Pod{namedPod("1")}},
			args:      args{},
			want: GroupedChangeSets{
				GroupedChangeSet{
					Definition: v1alpha1.DefaultFallbackGroupingDefinition,
					ChangeSet:  ChangeSet{ToKeep: []corev1.Pod{namedPod("1")}},
				},
			},
		},
		{
			name:      "non-matching group definitions should result in defaulted group being added",
			changeSet: ChangeSet{ToKeep: []corev1.Pod{namedPod("1")}},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					fooMatchingGroupingDefinition,
				},
			},
			want: GroupedChangeSets{
				GroupedChangeSet{
					Definition: fooMatchingGroupingDefinition,
					ChangeSet:  emptyChangeSet,
					PodsState:  newEmptyPodsState(),
				},
				GroupedChangeSet{
					Definition: v1alpha1.DefaultFallbackGroupingDefinition,
					ChangeSet:  ChangeSet{ToKeep: []corev1.Pod{namedPod("1")}},
				},
			},
		},
		{
			name: "pods should be bucketed into the groups based on the selector and include relevant PodsState",
			changeSet: ChangeSet{
				ToAdd:    []corev1.Pod{bazPod},
				ToKeep:   []corev1.Pod{fooPod},
				ToRemove: []corev1.Pod{barPod},
				ToAddContext: map[string]PodToAdd{
					bazPod.Name: bazPodToAdd,
				},
			},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					fooMatchingGroupingDefinition,
				},
				remainingPodsState: initializePodsState(PodsState{
					Pending:        map[string]corev1.Pod{fooPod.Name: fooPod},
					RunningJoining: map[string]corev1.Pod{barPod.Name: barPod},
				}),
			},
			want: GroupedChangeSets{
				GroupedChangeSet{
					Definition: fooMatchingGroupingDefinition,
					ChangeSet: ChangeSet{
						ToAdd:    []corev1.Pod{},
						ToKeep:   []corev1.Pod{fooPod},
						ToRemove: []corev1.Pod{},

						ToAddContext: map[string]PodToAdd{},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Name: fooPod},
					}),
				},
				GroupedChangeSet{
					Definition: v1alpha1.DefaultFallbackGroupingDefinition,
					ChangeSet: ChangeSet{
						ToKeep:   []corev1.Pod{},
						ToRemove: []corev1.Pod{barPod},
						ToAdd:    []corev1.Pod{bazPod},

						ToAddContext: map[string]PodToAdd{
							bazPod.Name: bazPodToAdd,
						},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{barPod.Name: barPod},
					}),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.changeSet
			got, err := s.Group(tt.args.groupingDefinitions, tt.args.remainingPodsState)
			if (err != nil) != tt.wantErr {
				t.Errorf("ChangeSet.Group() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_sortPodsByMasterNodeLastAndCreationTimestampAsc(t *testing.T) {
	masterNode := namedPodWithCreationTimestamp("master", time.Unix(5, 0))

	type args struct {
		masterNode *corev1.Pod
		pods       []corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want []corev1.Pod
	}{
		{
			name: "sample",
			args: args{
				masterNode: &masterNode,
				pods: []corev1.Pod{
					masterNode,
					namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
					namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
					namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				},
			},
			want: []corev1.Pod{
				namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
				namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
				namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				masterNode,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(
				tt.args.pods,
				sortPodsByMasterNodeLastAndCreationTimestampAsc(tt.args.masterNode, tt.args.pods),
			)

			assert.Equal(t, tt.want, tt.args.pods)
		})
	}
}

func Test_sortPodsByMasterNodesFirstThenNameAsc(t *testing.T) {
	masterNode5 := namedPodWithCreationTimestamp("master5", time.Unix(5, 0))
	masterNode5.Labels = NodeTypesMasterLabelName.AsMap(true)
	masterNode6 := namedPodWithCreationTimestamp("master6", time.Unix(6, 0))
	masterNode6.Labels = NodeTypesMasterLabelName.AsMap(true)

	type args struct {
		pods []corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want []corev1.Pod
	}{
		{
			name: "sample",
			args: args{
				pods: []corev1.Pod{
					namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
					masterNode6,
					namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
					masterNode5,
					namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				},
			},
			want: []corev1.Pod{
				masterNode5,
				masterNode6,
				namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
				namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
				namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(
				tt.args.pods,
				sortPodsByMasterNodesFirstThenNameAsc(tt.args.pods),
			)

			assert.Equal(t, tt.want, tt.args.pods)
		})
	}
}

func TestPerformableChanges_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		changes PerformableChanges
		want    bool
	}{
		{name: "empty", changes: PerformableChanges{}, want: true},
		{name: "creation", changes: PerformableChanges{ScheduleForCreation: []CreatablePod{{}}}, want: false},
		{name: "deletion", changes: PerformableChanges{ScheduleForDeletion: []corev1.Pod{{}}}, want: false},
		{
			name: "creation and deletion",
			changes: PerformableChanges{
				ScheduleForCreation: []CreatablePod{{}},
				ScheduleForDeletion: []corev1.Pod{{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.changes
			if got := c.IsEmpty(); got != tt.want {
				t.Errorf("PerformableChanges.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGroupedChangeSets_CalculatePerformableChanges(t *testing.T) {
	tests := []struct {
		name    string
		s       GroupedChangeSets
		want    *PerformableChanges
		wantErr bool
	}{
		{
			name: "empty",
			s:    GroupedChangeSets{},
			want: &PerformableChanges{},
		},
		{
			name: "can only add if unavailable budget is maxed out",
			s: GroupedChangeSets{
				GroupedChangeSet{
					Definition: v1alpha1.DefaultFallbackGroupingDefinition,
					ChangeSet: ChangeSet{
						ToAdd: []corev1.Pod{namedPod("1")},
						ToAddContext: map[string]PodToAdd{
							"1": {},
						},
						ToRemove: []corev1.Pod{namedPod("2")},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{"2": namedPod("2")},
					}),
				},
			},
			want: &PerformableChanges{
				ScheduleForCreation: []CreatablePod{
					{Pod: namedPod("1"), PodSpecContext: PodSpecContext{}},
				},
				MaxUnavailableGroups: []int{0},
			},
		},
		{
			name: "can only remove if surge budget is maxed out",
			s: GroupedChangeSets{
				GroupedChangeSet{
					Definition: v1alpha1.DefaultFallbackGroupingDefinition,
					ChangeSet: ChangeSet{
						ToAdd: []corev1.Pod{namedPod("1")},
						ToAddContext: map[string]PodToAdd{
							"1": {},
						},
						ToRemove: []corev1.Pod{namedPod("2")},
					},
					PodsState: initializePodsState(PodsState{
						RunningReady: map[string]corev1.Pod{"2": namedPod("2"), "3": namedPod("3")},
					}),
				},
			},
			want: &PerformableChanges{
				ScheduleForDeletion: []corev1.Pod{
					namedPod("2"),
				},
				MaxSurgeGroups: []int{0},
			},
		},
		{
			name: "can both remove and add up to the surge and unavailability budgets are exhausted",
			s: GroupedChangeSets{
				GroupedChangeSet{
					Definition: v1alpha1.GroupingDefinition{
						Selector: v1.LabelSelector{},
						Strategy: v1alpha1.GroupChangeStrategy{
							MaxSurge:       1,
							MaxUnavailable: 1,
						},
					},
					ChangeSet: ChangeSet{
						ToAdd: []corev1.Pod{namedPod("add-1"), namedPod("add-2")},
						ToAddContext: map[string]PodToAdd{
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
			want: &PerformableChanges{
				ScheduleForCreation: []CreatablePod{
					{Pod: namedPod("add-1"), PodSpecContext: PodSpecContext{}},
				},
				ScheduleForDeletion: []corev1.Pod{
					namedPod("remove-1"),
				},
				MaxSurgeGroups:       []int{0},
				MaxUnavailableGroups: []int{0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.CalculatePerformableChanges()
			if (err != nil) != tt.wantErr {
				t.Errorf("GroupedChangeSets.CalculatePerformableChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGroupedChangeSet_KeyNumbers(t *testing.T) {
	type fields struct {
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
					Strategy: v1alpha1.GroupChangeStrategy{
						MaxSurge:       1,
						MaxUnavailable: 1,
					},
				},
				ChangeSet: ChangeSet{
					ToAdd: []corev1.Pod{namedPod("add-1"), namedPod("add-2")},
					ToAddContext: map[string]PodToAdd{
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
				TargetPods:             3,
				CurrentPods:            3,
				CurrentSurge:           0,
				CurrentOperationalPods: 3,
				CurrentUnavailable:     0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := GroupedChangeSet{
				Definition: tt.fields.Definition,
				ChangeSet:  tt.fields.ChangeSet,
				PodsState:  tt.fields.PodsState,
			}

			assert.Equal(t, tt.want, s.KeyNumbers())
		})
	}
}
