// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Resources struct {
	Limits   ResourceAllocations `json:"limits,omitempty"`
	Requests ResourceAllocations `json:"requests,omitempty"`
}

// IsEmpty reports whether the shorthand contains no CPU or memory values on
// either Requests or Limits.
func (r Resources) IsEmpty() bool {
	return r.Limits.CPU == nil && r.Limits.Memory == nil &&
		r.Requests.CPU == nil && r.Requests.Memory == nil
}

// ResourceAllocations holds optional CPU and memory quantities that the
// shorthand Resources field applies to the main container's Requests or Limits.
// Using pointers lets callers distinguish "no override" (nil) from "override
// with a zero quantity" (non-nil pointing to a zero value).
type ResourceAllocations struct {
	// CPU overrides the main container's CPU request/limit when the parent Resources
	// is merged into a PodTemplate. A nil value means "do not override": any CPU
	// value already set on the main container in the PodTemplate is passed through
	// unchanged. Setting this field to nil does not unset a CPU value present in
	// the PodTemplate; to remove it, edit the PodTemplate's container resources.
	// A non-nil value wins over the PodTemplate's CPU, including an explicit zero quantity.
	CPU *resource.Quantity `json:"cpu,omitempty"`
	// Memory overrides the main container's memory request/limit when the parent
	// Resources is merged into a PodTemplate. A nil value means "do not override":
	// any memory value already set on the main container in the PodTemplate is
	// passed through unchanged. Setting this field to nil does not unset a memory
	// value present in the PodTemplate; to remove it, edit the PodTemplate's
	// container resources.
	// A non-nil value wins over the PodTemplate's memory, including an explicit zero quantity.
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// ToResourceList converts the CPU/memory allocations into a corev1.ResourceList.
// Returns nil when both CPU and memory are unset so callers can treat absent
// allocations as "no value". Quantities are deep-copied so the returned map
// shares no internal state with the original ResourceAllocations.
func (ra *ResourceAllocations) ToResourceList() corev1.ResourceList {
	if ra == nil || (ra.CPU == nil && ra.Memory == nil) {
		return nil
	}
	out := make(corev1.ResourceList, 2)
	if ra.CPU != nil {
		out[corev1.ResourceCPU] = ra.CPU.DeepCopy()
	}
	if ra.Memory != nil {
		out[corev1.ResourceMemory] = ra.Memory.DeepCopy()
	}
	return out
}
