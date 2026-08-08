// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// countingClient records the patches issued so tests can assert that PatchAnnotations sends the
// minimal diff, and nothing at all when there is no diff.
type countingClient struct {
	k8s.Client
	patches []string
}

func (c *countingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	c.patches = append(c.patches, string(data))
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func newPod(annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:        "p",
		Namespace:   "ns",
		Annotations: annotations,
	}}
}

func TestPatchAnnotations(t *testing.T) {
	for _, tt := range []struct {
		name        string
		existing    map[string]string
		set         map[string]string
		remove      []string
		wantPatches int
		wantFinal   map[string]string
	}{
		{
			name:        "adds a key without touching the others",
			existing:    map[string]string{"keep": "yes"},
			set:         map[string]string{"added": "1"},
			wantPatches: 1,
			wantFinal:   map[string]string{"keep": "yes", "added": "1"},
		},
		{
			name:        "sets an annotation on an object that has none",
			set:         map[string]string{"added": "1"},
			wantPatches: 1,
			wantFinal:   map[string]string{"added": "1"},
		},
		{
			name:        "updates a changed value",
			existing:    map[string]string{"a": "old", "keep": "yes"},
			set:         map[string]string{"a": "new"},
			wantPatches: 1,
			wantFinal:   map[string]string{"a": "new", "keep": "yes"},
		},
		{
			name:        "removes a key",
			existing:    map[string]string{"gone": "1", "keep": "yes"},
			remove:      []string{"gone"},
			wantPatches: 1,
			wantFinal:   map[string]string{"keep": "yes"},
		},
		{
			// The hot path: this runs on every reconcile, so it must not generate API traffic.
			name:        "no request when the value already matches",
			existing:    map[string]string{"a": "same"},
			set:         map[string]string{"a": "same"},
			wantPatches: 0,
			wantFinal:   map[string]string{"a": "same"},
		},
		{
			name:        "no request when removing an absent key",
			existing:    map[string]string{"keep": "yes"},
			remove:      []string{"never-there"},
			wantPatches: 0,
			wantFinal:   map[string]string{"keep": "yes"},
		},
		{
			name:        "no request when the object has no annotations and nothing is asked",
			wantPatches: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := newPod(tt.existing)
			c := &countingClient{Client: k8s.NewFakeClient(pod)}

			require.NoError(t, PatchAnnotations(context.Background(), c, pod, tt.set, tt.remove...))
			assert.Len(t, c.patches, tt.wantPatches)

			var live corev1.Pod
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "p"}, &live))
			assert.Equal(t, tt.wantFinal, live.Annotations)
		})
	}
}

// TestPatchAnnotations_TouchesOnlyAnnotations is the point of the whole helper: the request body
// must not carry the rest of the object. A full-object Update is what made the operator claim
// ownership of spec fields it does not manage, breaking `helm upgrade` under Server-Side Apply.
func TestPatchAnnotations_TouchesOnlyAnnotations(t *testing.T) {
	pod := newPod(map[string]string{"keep": "yes"})
	pod.Spec.Containers = []corev1.Container{{Name: "c", Image: "img"}}
	c := &countingClient{Client: k8s.NewFakeClient(pod)}

	require.NoError(t, PatchAnnotations(context.Background(), c, pod, map[string]string{"added": "1"}))

	require.Len(t, c.patches, 1)
	assert.NotContains(t, c.patches[0], "spec")
	assert.NotContains(t, c.patches[0], "containers")
	// Only the changed key travels, not the annotations the caller left alone.
	assert.Contains(t, c.patches[0], "added")
	assert.NotContains(t, c.patches[0], "keep")
	// resourceVersion preserves the optimistic concurrency guarantee Update gave us.
	assert.Contains(t, c.patches[0], "resourceVersion")
}
