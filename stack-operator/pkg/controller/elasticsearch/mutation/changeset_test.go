package mutation

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"

	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func namedPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
	}
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
	examplePodToAdd := support.PodToAdd{PodSpecCtx: support.PodSpecContext{PodSpec: corev1.PodSpec{Hostname: "foo"}}}
	type args struct {
		changes    support.Changes
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
				ToAddContext: map[string]support.PodToAdd{},
			},
			wantErr: false,
		},
		{
			name: "with pods",
			args: args{
				changes: support.Changes{
					ToAdd: []support.PodToAdd{
						examplePodToAdd,
					},
					ToKeep:   []corev1.Pod{namedPod("2")},
					ToRemove: []corev1.Pod{namedPod("3")},
				},
				newPodFunc: func(ctx support.PodSpecContext) (corev1.Pod, error) {
					if !reflect.DeepEqual(ctx, examplePodToAdd.PodSpecCtx) {
						return corev1.Pod{}, fmt.Errorf("got unexpected parameter: %v", ctx)
					}
					return namedPod("example"), nil
				},
			},
			want: &ChangeSet{
				ToAdd:        []corev1.Pod{namedPod("example")},
				ToAddContext: map[string]support.PodToAdd{"example": examplePodToAdd},

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

func TestChangeSet_Group(t *testing.T) {
	fooMatchingGroupingDefinition := v1alpha1.GroupingDefinition{
		Selector: v1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
	}

	fooPod := withLabels(namedPod("1"), map[string]string{"foo": "bar"})

	barPod := withLabels(namedPod("2"), map[string]string{"bar": "bar"})
	bazPod := withLabels(namedPod("3"), map[string]string{"baz": "bar"})
	bazPodToAdd := support.PodToAdd{
		PodSpecCtx: support.PodSpecContext{PodSpec: corev1.PodSpec{Hostname: "baz"}},
	}

	foobarPod := withLabels(namedPod("4"), map[string]string{"foo": "bar", "bar": "baz"})

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
			name:      "no group definitions should result in a defaulted group",
			changeSet: ChangeSet{ToKeep: []corev1.Pod{namedPod("1")}},
			args:      args{},
			want: GroupedChangeSets{
				GroupedChangeSet{
					Name: UnmatchedGroupName,
					ChangeSet: ChangeSet{
						ToKeep:       []corev1.Pod{namedPod("1")},
						ToAddContext: map[string]support.PodToAdd{},
					},
				},
			},
		},
		{
			name:      "non-matching group definitions should result in a defaulted group",
			changeSet: ChangeSet{ToKeep: []corev1.Pod{namedPod("1")}},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					fooMatchingGroupingDefinition,
				},
			},
			want: GroupedChangeSets{
				GroupedChangeSet{
					Name: UnmatchedGroupName,
					ChangeSet: ChangeSet{
						ToKeep:       []corev1.Pod{namedPod("1")},
						ToAdd:        []corev1.Pod{},
						ToRemove:     []corev1.Pod{},
						ToAddContext: map[string]support.PodToAdd{},
					},
				},
			},
		},
		{
			name: "pods should be bucketed into the groups based on the selector and include relevant PodsState",
			changeSet: ChangeSet{
				ToAdd:    []corev1.Pod{bazPod},
				ToKeep:   []corev1.Pod{fooPod},
				ToRemove: []corev1.Pod{barPod},
				ToAddContext: map[string]support.PodToAdd{
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
					Name: indexedGroupName(0),
					ChangeSet: ChangeSet{
						ToAdd:    []corev1.Pod{},
						ToKeep:   []corev1.Pod{fooPod},
						ToRemove: []corev1.Pod{},

						ToAddContext: map[string]support.PodToAdd{},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Name: fooPod},
					}),
				},
				GroupedChangeSet{
					Name: UnmatchedGroupName,
					ChangeSet: ChangeSet{
						ToKeep:   []corev1.Pod{},
						ToRemove: []corev1.Pod{barPod},
						ToAdd:    []corev1.Pod{bazPod},

						ToAddContext: map[string]support.PodToAdd{
							bazPod.Name: bazPodToAdd,
						},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{barPod.Name: barPod},
					}),
				},
			},
		},
		{
			name: "should match when there are multiple labels",
			changeSet: ChangeSet{
				ToAdd:    []corev1.Pod{bazPod},
				ToKeep:   []corev1.Pod{fooPod},
				ToRemove: []corev1.Pod{foobarPod},
				ToAddContext: map[string]support.PodToAdd{
					bazPod.Name: bazPodToAdd,
				},
			},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					{
						Selector: v1.LabelSelector{
							MatchLabels: map[string]string{
								"foo": "bar",
								"bar": "baz",
							},
						},
					},
				},
				remainingPodsState: initializePodsState(PodsState{
					Pending:        map[string]corev1.Pod{fooPod.Name: fooPod},
					RunningJoining: map[string]corev1.Pod{foobarPod.Name: foobarPod},
				}),
			},
			want: GroupedChangeSets{
				GroupedChangeSet{
					Name: indexedGroupName(0),
					ChangeSet: ChangeSet{
						ToAdd:    []corev1.Pod{},
						ToKeep:   []corev1.Pod{},
						ToRemove: []corev1.Pod{foobarPod},

						ToAddContext: map[string]support.PodToAdd{},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{foobarPod.Name: foobarPod},
					}),
				},
				GroupedChangeSet{
					Name: UnmatchedGroupName,
					ChangeSet: ChangeSet{
						ToKeep:   []corev1.Pod{fooPod},
						ToRemove: []corev1.Pod{},
						ToAdd:    []corev1.Pod{bazPod},

						ToAddContext: map[string]support.PodToAdd{
							bazPod.Name: bazPodToAdd,
						},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Name: fooPod},
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
