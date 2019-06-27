// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var defaultPodWithConfig = ESPodWithConfig(defaultImage, defaultCPULimit)
var defaultPodToDelete = PodToDelete{PodWithConfig: defaultPodWithConfig}
var emptyPodWithConfig = pod.PodWithConfig{Pod: corev1.Pod{}}
var defaultPodSpecCtx = ESPodSpecContext(defaultImage, defaultCPULimit)

func namedPod(name string) pod.PodWithConfig {
	return pod.PodWithConfig{
		Pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
		Config: settings.CanonicalConfig{},
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
		Config: settings.CanonicalConfig{},
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
		ToDelete PodsToDelete
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
				ToDelete: PodsToDelete{{PodWithConfig: emptyPodWithConfig}},
			},
			want: true,
		},
		{
			name: "create and delete has changes",
			fields: fields{
				ToCreate: []PodToCreate{PodToCreate{}},
				ToDelete: PodsToDelete{{PodWithConfig: emptyPodWithConfig}},
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
		ToDelete PodsToDelete
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
				ToDelete: PodsToDelete{},
			},
			want: true,
		},
		{
			name: "with pod to create should not be empty",
			fields: fields{
				ToCreate: []PodToCreate{{}},
				ToKeep:   pod.PodsWithConfig{},
				ToDelete: PodsToDelete{},
			},
			want: false,
		},
		{
			name: "with pod to keep not be empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   pod.PodsWithConfig{{}},
				ToDelete: PodsToDelete{},
			},
			want: false,
		},
		{
			name: "with pod to delete should not empty",
			fields: fields{
				ToCreate: []PodToCreate{},
				ToKeep:   pod.PodsWithConfig{},
				ToDelete: PodsToDelete{{}},
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
		PodSpecCtx: pod.PodSpecContext{PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Hostname: "baz"}}},
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
						ToCreate: PodsToCreate{},
						ToDelete: PodsToDelete{},
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
						ToCreate: PodsToCreate{},
						ToDelete: PodsToDelete{},
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
				ToDelete: PodsToDelete{{PodWithConfig: barPod}},
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
						ToDelete: PodsToDelete{},
					},
					PodsState: initializePodsState(PodsState{
						Pending: map[string]corev1.Pod{fooPod.Pod.Name: fooPod.Pod},
					}),
				},
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   pod.PodsWithConfig{},
						ToDelete: PodsToDelete{{PodWithConfig: barPod}},
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
				ToDelete: PodsToDelete{{PodWithConfig: foobarPod}},
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
						ToDelete: PodsToDelete{{PodWithConfig: foobarPod}},
					},
					PodsState: initializePodsState(PodsState{
						RunningJoining: map[string]corev1.Pod{foobarPod.Pod.Name: foobarPod.Pod},
					}),
				},
				ChangeGroup{
					Name: UnmatchedGroupName,
					Changes: Changes{
						ToKeep:   pod.PodsWithConfig{fooPod},
						ToDelete: PodsToDelete{},
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

func TestChanges_Partition(t *testing.T) {
	label1 := map[string]string{
		"key": "value1",
	}
	label2 := map[string]string{
		"key": "value2",
	}
	podWithLabel1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "podWithLabel1",
			Labels: label1,
		},
	}
	toKeep1 := pod.PodWithConfig{Pod: podWithLabel1}
	toCreate1 := PodToCreate{Pod: podWithLabel1}
	toDelete1 := PodToDelete{PodWithConfig: pod.PodWithConfig{Pod: podWithLabel1}}
	podWithLabel2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "podWithLabel2",
			Labels: label2,
		},
	}
	toKeep2 := pod.PodWithConfig{Pod: podWithLabel2}
	toCreate2 := PodToCreate{Pod: podWithLabel2}
	toDelete2 := PodToDelete{PodWithConfig: pod.PodWithConfig{Pod: podWithLabel2}}
	tests := []struct {
		name     string
		selector labels.Selector
		changes  Changes
		want1    Changes
		want2    Changes
	}{
		{
			name:     "sample changes",
			selector: labels.SelectorFromSet(label1),
			changes: Changes{
				ToKeep:   pod.PodsWithConfig{toKeep1, toKeep1, toKeep2},
				ToCreate: PodsToCreate{toCreate1, toCreate1, toCreate2},
				ToDelete: PodsToDelete{toDelete1, toDelete1, toDelete1, toDelete2, toDelete2},
			},
			want1: Changes{
				ToKeep:   pod.PodsWithConfig{toKeep1, toKeep1},
				ToCreate: PodsToCreate{toCreate1, toCreate1},
				ToDelete: PodsToDelete{toDelete1, toDelete1, toDelete1},
			},
			want2: Changes{
				ToKeep:   pod.PodsWithConfig{toKeep2},
				ToCreate: PodsToCreate{toCreate2},
				ToDelete: PodsToDelete{toDelete2, toDelete2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1, got2 := tt.changes.Partition(tt.selector)
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("Changes.Partition() got = %v, want %v", got1, tt.want1)
			}
			if !reflect.DeepEqual(got2, tt.want2) {
				t.Errorf("Changes.Partition() got1 = %v, want %v", got2, tt.want2)
			}
		})
	}
}
