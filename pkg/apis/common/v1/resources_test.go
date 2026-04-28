// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestResourceAllocationsToResourceList(t *testing.T) {
	cpu1 := resource.MustParse("1")
	mem2Gi := resource.MustParse("2Gi")
	cpu1Copy := cpu1.DeepCopy()
	mem2GiCopy := mem2Gi.DeepCopy()

	t.Parallel()
	tests := []struct {
		name string
		ra   *ResourceAllocations
		want corev1.ResourceList
	}{
		{
			name: "nil receiver",
			ra:   nil,
			want: nil,
		},
		{
			name: "empty",
			ra:   &ResourceAllocations{},
			want: nil,
		},
		{
			name: "cpu only",
			ra:   &ResourceAllocations{CPU: &cpu1},
			want: corev1.ResourceList{corev1.ResourceCPU: cpu1Copy},
		},
		{
			name: "memory only",
			ra:   &ResourceAllocations{Memory: &mem2Gi},
			want: corev1.ResourceList{corev1.ResourceMemory: mem2GiCopy},
		},
		{
			name: "cpu and memory",
			ra:   &ResourceAllocations{CPU: &cpu1, Memory: &mem2Gi},
			want: corev1.ResourceList{
				corev1.ResourceCPU:    cpu1Copy,
				corev1.ResourceMemory: mem2GiCopy,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.ra.ToResourceList()
			if len(got) != len(tt.want) {
				t.Fatalf("ToResourceList() len = %d, want %d", len(got), len(tt.want))
			}
			for name, wantQty := range tt.want {
				gotQty, ok := got[name]
				if !ok {
					t.Fatalf("missing key %q in result", name)
				}
				if !gotQty.Equal(wantQty) {
					t.Errorf("key %q: got %v want %v", name, gotQty, wantQty)
				}
			}
		})
	}
}

// TestResourcesJSONMarshalOmitsZeroValue verifies that a zero-valued Resources
// (no CPU or memory set on either Limits or Requests) marshals to an empty JSON
// object so that, when embedded in a parent type with `,omitzero`, the field is
// omitted entirely. This guards against the regression where non-autoscaled
// NodeSets would persist as `resources: { limits: {}, requests: {} }` after the
// operator round-tripped the CR (see PR #9346 review feedback).
func TestResourcesJSONMarshalOmitsZeroValue(t *testing.T) {
	cpu := resource.MustParse("1")
	mem := resource.MustParse("2Gi")

	type wrapper struct {
		Resources Resources `json:"resources,omitzero"`
	}

	t.Parallel()
	tests := []struct {
		name string
		in   wrapper
		want string
	}{
		{
			name: "zero value is omitted from parent",
			in:   wrapper{},
			want: `{}`,
		},
		{
			name: "explicit empty struct is also omitted",
			in:   wrapper{Resources: Resources{Limits: ResourceAllocations{}, Requests: ResourceAllocations{}}},
			want: `{}`,
		},
		{
			name: "requests cpu only emits requests, omits limits",
			in:   wrapper{Resources: Resources{Requests: ResourceAllocations{CPU: &cpu}}},
			want: `{"resources":{"requests":{"cpu":"1"}}}`,
		},
		{
			name: "limits memory only emits limits, omits requests",
			in:   wrapper{Resources: Resources{Limits: ResourceAllocations{Memory: &mem}}},
			want: `{"resources":{"limits":{"memory":"2Gi"}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestResourcesJSONRoundTripStripsLegacyEmptyStub verifies that a CR that
// already carries the empty stub on disk (e.g. written by an earlier operator
// version) is normalized to an absent `resources` key on the next operator-
// driven write. This is what allows existing clusters to self-clean on the next
// CR update.
func TestResourcesJSONRoundTripStripsLegacyEmptyStub(t *testing.T) {
	t.Parallel()
	type wrapper struct {
		Resources Resources `json:"resources,omitzero"`
	}
	// Simulate a CR persisted by the previous operator that emitted
	// "resources":{"limits":{},"requests":{}}.
	in := []byte(`{"resources":{"limits":{},"requests":{}}}`)
	var w wrapper
	if err := json.Unmarshal(in, &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	out, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(out) != `{}` {
		t.Errorf("legacy empty stub not normalized: got %s, want {}", out)
	}
}

func TestResourceAllocationsToResourceListDeepCopy(t *testing.T) {
	t.Parallel()
	q := resource.MustParse("100m")
	ra := &ResourceAllocations{CPU: &q}
	out := ra.ToResourceList()
	q.SetMilli(200)
	if out[corev1.ResourceCPU].Equal(q) {
		t.Error("ToResourceList should not share mutable state with source Quantity")
	}
}
