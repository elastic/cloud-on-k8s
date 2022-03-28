// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package expectations

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func newPod(name string, uuid types.UID) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
			UID:       uuid,
		},
	}
}

func TestExpectedPodDeletions_DeletionsSatisfied(t *testing.T) {
	pod1 := newPod("pod1", uuid.NewUUID())
	pod1Recreated := newPod("pod1", uuid.NewUUID()) // same name, different UID
	pod2 := newPod("pod2", uuid.NewUUID())

	tests := []struct {
		name                  string
		resources             []runtime.Object
		expectDeletions       []corev1.Pod
		wantSatisfied         bool
		wantExpectedDeletions map[types.NamespacedName]types.UID
	}{
		{
			name:                  "no deletion expected",
			resources:             []runtime.Object{&pod1, &pod2},
			expectDeletions:       nil,
			wantSatisfied:         true,
			wantExpectedDeletions: map[types.NamespacedName]types.UID{},
		},
		{
			name:                  "one deletion expected, unsatisfied",
			resources:             []runtime.Object{&pod1, &pod2},
			expectDeletions:       []corev1.Pod{pod2},
			wantSatisfied:         false,
			wantExpectedDeletions: map[types.NamespacedName]types.UID{k8s.ExtractNamespacedName(&pod2): pod2.UID},
		},
		{
			name:                  "one deletion expected, satisfied",
			resources:             []runtime.Object{&pod1},
			expectDeletions:       []corev1.Pod{pod2},
			wantSatisfied:         true,
			wantExpectedDeletions: map[types.NamespacedName]types.UID{},
		},
		{
			name:            "two deletions expected, only one satisfied",
			resources:       []runtime.Object{&pod2},
			expectDeletions: []corev1.Pod{pod1, pod2},
			// still waiting for pod2 deletion to happen
			wantSatisfied:         false,
			wantExpectedDeletions: map[types.NamespacedName]types.UID{k8s.ExtractNamespacedName(&pod2): pod2.UID},
		},
		{
			name:                  "two deletions expected, satisfied",
			resources:             []runtime.Object{},
			expectDeletions:       []corev1.Pod{pod1, pod2},
			wantSatisfied:         true,
			wantExpectedDeletions: map[types.NamespacedName]types.UID{},
		},
		{
			name:                  "pod recreated with a different UID: deletion satisfied",
			resources:             []runtime.Object{&pod1Recreated},
			expectDeletions:       []corev1.Pod{pod1},
			wantSatisfied:         true,
			wantExpectedDeletions: map[types.NamespacedName]types.UID{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controllerscheme.SetupScheme()
			client := k8s.NewFakeClient(tt.resources...)
			e := NewExpectedPodDeletions(client)
			for i := range tt.expectDeletions {
				e.ExpectDeletion(tt.expectDeletions[i])
				require.Contains(t, e.podDeletions, k8s.ExtractNamespacedName(&tt.expectDeletions[i]))
			}
			pendingPodsDeletions, err := e.PendingPodDeletions()
			require.NoError(t, err)
			require.Equal(t, tt.wantSatisfied, len(pendingPodsDeletions) == 0)
			require.Equal(t, tt.wantExpectedDeletions, e.podDeletions)
		})
	}
}
