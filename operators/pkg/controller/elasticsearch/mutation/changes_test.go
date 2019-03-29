// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultPodWithConfig = ESPodWithConfig(defaultImage, defaultCPULimit)
var emptyPodWithConfig = pod.PodWithConfig{Pod: corev1.Pod{}, Config: settings.FlatConfig{}}
var defaultPodSpecCtx = ESPodSpecContext(defaultImage, defaultCPULimit)

func namedPod(name string) pod.PodWithConfig {
	return pod.PodWithConfig{
		Pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
		Config: nil,
	}
}

func namedPodWithCreationTimestamp(name string, creationTimestamp time.Time) pod.PodWithConfig {
	return pod.PodWithConfig{
		Pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				CreationTimestamp: metav1.Time{Time: creationTimestamp},
			},
		},
		Config: nil,
	}
}

func withLabels(p pod.PodWithConfig, labels map[string]string) pod.PodWithConfig {
	p.Pod.Labels = labels
	return p
}

func TestChanges_HasChanges(t *testing.T) {
	type fields struct {
		ToCreate []PodToCreate
		ToKeep   pod.PodsWithConfig
		ToDelete pod.PodsWithConfig
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
				ToKeep: pod.PodsWithConfig{emptyPodWithConfig},
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
				ToDelete: pod.PodsWithConfig{emptyPodWithConfig},
			},
			want: true,
		},
		{
			name: "create and delete has changes",
			fields: fields{
				ToCreate: []PodToCreate{PodToCreate{}},
				ToDelete: pod.PodsWithConfig{emptyPodWithConfig},
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
		ToKeep   pod.PodsWithConfig
		ToDelete pod.PodsWithConfig
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
				ToKeep:   pod.PodsWithConfig{},
				ToDelete: pod.PodsWithConfig{},
			},
			want: true,
		},
		{
			name: "with pod to create should not be empty",
			fields: fields{
				ToCreate: []PodToCreate{{}},
				ToKeep:   pod.PodsWithConfig{},
				ToDelete: pod.PodsWithConfig{},
			},
			want: false,
		},
		{
			name: "with pod to keep not be empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   pod.PodsWithConfig{{}},
				ToDelete: pod.PodsWithConfig{},
			},
			want: false,
		},
		{
			name: "with pod to delete should not empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   pod.PodsWithConfig{},
				ToDelete: pod.PodsWithConfig{{}},
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
		Pod:        withLabels(namedPod("3"), map[string]string{"baz": "bar"}).Pod,
		PodSpecCtx: pod.PodSpecContext{PodSpec: corev1.PodSpec{Hostname: "baz"}},
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
			changes: Changes{ToKeep: pod.PodsWithConfig{namedPod("1")}},
			args: args{
				remainingPodsState: NewEmptyPodsState(),
			},
			want: ChangeGroups{
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   pod.PodsWithConfig{namedPod("1")},
						ToCreate: []PodToCreate{},
						ToDelete: pod.PodsWithConfig{},
					},
					PodsState: NewEmptyPodsState(),
				},
			},
		},
		{
			name:    "non-matching group definitions should result in a defaulted group",
			changes: Changes{ToKeep: pod.PodsWithConfig{namedPod("1")}},
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
						ToKeep:   pod.PodsWithConfig{namedPod("1")},
						ToCreate: []PodToCreate{},
						ToDelete: pod.PodsWithConfig{},
					},
					PodsState: NewEmptyPodsState(),
				},
			},
		},
		{
			name: "pods should be bucketed into the groups based on the selector and include relevant PodsState",
			changes: Changes{
				ToCreate: []PodToCreate{bazPodToCreate},
				ToKeep:   pod.PodsWithConfig{fooPod},
				ToDelete: pod.PodsWithConfig{barPod},
			},
			args: args{
				groupingDefinitions: []v1alpha1.GroupingDefinition{
					fooMatchingGroupingDefinition,
				},
				remainingPodsState: initializePodsState(PodsState{
					Pending:        map[string]corev1.Pod{fooPod.Pod.Name: fooPod.Pod},
					RunningJoining: map[string]corev1.Pod{barPod.Pod.Name: barPod.Pod},
				}),
			},
			want: ChangeGroups{
				ChangeGroup{
					Name: indexedGroupName(0),
					Changes: Changes{
						ToCreate: []PodToCreate{},
						ToKeep:   pod.PodsWithConfig{fooPod},
						ToDelete: pod.PodsWithConfig{},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Pod.Name: fooPod.Pod},
					}),
				},
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   pod.PodsWithConfig{},
						ToDelete: pod.PodsWithConfig{barPod},
						ToCreate: []PodToCreate{bazPodToCreate},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{barPod.Pod.Name: barPod.Pod},
					}),
				},
			},
		},
		{
			name: "should match when there are multiple labels",
			changes: Changes{
				ToCreate: []PodToCreate{bazPodToCreate},
				ToKeep:   pod.PodsWithConfig{fooPod},
				ToDelete: pod.PodsWithConfig{foobarPod},
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
					Pending:        map[string]corev1.Pod{fooPod.Pod.Name: fooPod.Pod},
					RunningJoining: map[string]corev1.Pod{foobarPod.Pod.Name: foobarPod.Pod},
				}),
			},
			want: ChangeGroups{
				ChangeGroup{
					Name: indexedGroupName(0),
					Changes: Changes{
						ToCreate: []PodToCreate{},
						ToKeep:   pod.PodsWithConfig{},
						ToDelete: pod.PodsWithConfig{foobarPod},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{foobarPod.Pod.Name: foobarPod.Pod},
					}),
				},
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   pod.PodsWithConfig{fooPod},
						ToDelete: pod.PodsWithConfig{},
						ToCreate: []PodToCreate{bazPodToCreate},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Pod.Name: fooPod.Pod},
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
