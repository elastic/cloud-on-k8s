package mutation

import (
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultPod = ESPod(defaultImage, defaultCPULimit)
var defaultPodSpecCtx = ESPodSpecContext(defaultImage, defaultCPULimit)

func namedPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func namedPodWithCreationTimestamp(name string, creationTimestamp time.Time) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{Time: creationTimestamp},
		},
	}
}

func withLabels(pod corev1.Pod, labels map[string]string) corev1.Pod {
	pod.Labels = labels
	return pod
}

func TestChanges_HasChanges(t *testing.T) {
	type fields struct {
		ToCreate []PodToCreate
		ToKeep   []corev1.Pod
		ToDelete []corev1.Pod
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "empty has no changes",
			fields: fields{},
			want:   false,
		},
		{
			name: "something to keep still has no changes",
			fields: fields{
				ToKeep: []corev1.Pod{corev1.Pod{}},
			},
			want: false,
		},
		{
			name: "something to create has changes",
			fields: fields{
				ToCreate: []PodToCreate{PodToCreate{}},
			},
			want: true,
		},
		{
			name: "something to delete has changes",
			fields: fields{
				ToDelete: []corev1.Pod{corev1.Pod{}},
			},
			want: true,
		},
		{
			name: "create and delete has changes",
			fields: fields{
				ToCreate: []PodToCreate{PodToCreate{}},
				ToDelete: []corev1.Pod{corev1.Pod{}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Changes{
				ToCreate: tt.fields.ToCreate,
				ToKeep:   tt.fields.ToKeep,
				ToDelete: tt.fields.ToDelete,
			}
			if got := c.HasChanges(); got != tt.want {
				t.Errorf("Changes.HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChanges_IsEmpty(t *testing.T) {
	type fields struct {
		ToCreate []PodToCreate
		ToKeep   []corev1.Pod
		ToDelete []corev1.Pod
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "no inner list should be empty",
			fields: fields{},
			want:   true,
		},
		{
			name: "empty inner lists should be empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   []corev1.Pod{},
				ToDelete: []corev1.Pod{},
			},
			want: true,
		},
		{
			name: "with pod to create should not be empty",
			fields: fields{
				ToCreate: []PodToCreate{{}},
				ToKeep:   []corev1.Pod{},
				ToDelete: []corev1.Pod{},
			},
			want: false,
		},
		{
			name: "with pod to keep not be empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   []corev1.Pod{{}},
				ToDelete: []corev1.Pod{},
			},
			want: false,
		},
		{
			name: "with pod to delete should not empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   []corev1.Pod{},
				ToDelete: []corev1.Pod{{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Changes{
				ToCreate: tt.fields.ToCreate,
				ToKeep:   tt.fields.ToKeep,
				ToDelete: tt.fields.ToDelete,
			}
			if got := c.IsEmpty(); got != tt.want {
				t.Errorf("Changes.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChanges_Group(t *testing.T) {
	fooMatchingGroupingDefinition := v1alpha1.GroupingDefinition{
		Selector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
	}

	fooPod := withLabels(namedPod("1"), map[string]string{"foo": "bar"})
	barPod := withLabels(namedPod("2"), map[string]string{"bar": "bar"})
	bazPodToCreate := PodToCreate{
		Pod:        withLabels(namedPod("3"), map[string]string{"baz": "bar"}),
		PodSpecCtx: support.PodSpecContext{PodSpec: corev1.PodSpec{Hostname: "baz"}},
	}

	foobarPod := withLabels(namedPod("4"), map[string]string{"foo": "bar", "bar": "baz"})

	type args struct {
		groupingDefinitions []v1alpha1.GroupingDefinition
		remainingPodsState  PodsState
	}
	tests := []struct {
		name    string
		changes Changes
		args    args
		want    ChangeGroups
		wantErr bool
	}{
		{
			name:    "empty",
			changes: Changes{},
			args: args{
				remainingPodsState: NewEmptyPodsState()},
			want: ChangeGroups{},
		},
		{
			name:    "no group definitions should result in a defaulted group",
			changes: Changes{ToKeep: []corev1.Pod{namedPod("1")}},
			args: args{
				remainingPodsState: NewEmptyPodsState(),
			},
			want: ChangeGroups{
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   []corev1.Pod{namedPod("1")},
						ToCreate: []PodToCreate{},
						ToDelete: []corev1.Pod{},
					},
					PodsState: NewEmptyPodsState(),
				},
			},
		},
		{
			name:    "non-matching group definitions should result in a defaulted group",
			changes: Changes{ToKeep: []corev1.Pod{namedPod("1")}},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					fooMatchingGroupingDefinition,
				},
				remainingPodsState: NewEmptyPodsState(),
			},
			want: ChangeGroups{
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   []corev1.Pod{namedPod("1")},
						ToCreate: []PodToCreate{},
						ToDelete: []corev1.Pod{},
					},
					PodsState: NewEmptyPodsState(),
				},
			},
		},
		{
			name: "pods should be bucketed into the groups based on the selector and include relevant PodsState",
			changes: Changes{
				ToCreate: []PodToCreate{bazPodToCreate},
				ToKeep:   []corev1.Pod{fooPod},
				ToDelete: []corev1.Pod{barPod},
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
			want: ChangeGroups{
				ChangeGroup{
					Name: indexedGroupName(0),
					Changes: Changes{
						ToCreate: []PodToCreate{},
						ToKeep:   []corev1.Pod{fooPod},
						ToDelete: []corev1.Pod{},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Name: fooPod},
					}),
				},
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   []corev1.Pod{},
						ToDelete: []corev1.Pod{barPod},
						ToCreate: []PodToCreate{bazPodToCreate},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{barPod.Name: barPod},
					}),
				},
			},
		},
		{
			name: "should match when there are multiple labels",
			changes: Changes{
				ToCreate: []PodToCreate{bazPodToCreate},
				ToKeep:   []corev1.Pod{fooPod},
				ToDelete: []corev1.Pod{foobarPod},
			},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					{
						Selector: metav1.LabelSelector{
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
			want: ChangeGroups{
				ChangeGroup{
					Name: indexedGroupName(0),
					Changes: Changes{
						ToCreate: []PodToCreate{},
						ToKeep:   []corev1.Pod{},
						ToDelete: []corev1.Pod{foobarPod},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{foobarPod.Name: foobarPod},
					}),
				},
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   []corev1.Pod{fooPod},
						ToDelete: []corev1.Pod{},
						ToCreate: []PodToCreate{bazPodToCreate},
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
			s := tt.changes
			got, err := s.Group(tt.args.groupingDefinitions, tt.args.remainingPodsState)
			if (err != nil) != tt.wantErr {
				t.Errorf("Changes.Group() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
