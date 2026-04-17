// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type Resources struct {
	Limits   ResourceAllocations `json:"limits,omitempty"`
	Requests ResourceAllocations `json:"requests,omitempty"`
}

type ResourceAllocations struct {
	// CPU overrides the main container's CPU request/limit when the parent Resources
	// is merged into a PodTemplate. A nil value means "do not override": any CPU
	// value already set on the main container in the PodTemplate is passed through
	// unchanged. Setting this field to nil does not unset a CPU value present in
	// the PodTemplate; to remove it, edit the PodTemplate's container resources.
	// Setting this field to the literal "0" sets the override to a zero quantity
	// and does not clear the PodTemplate's CPU.
	CPU *resource.Quantity `json:"cpu,omitempty"`
	// Memory overrides the main container's memory request/limit when the parent
	// Resources is merged into a PodTemplate. A nil value means "do not override":
	// any memory value already set on the main container in the PodTemplate is
	// passed through unchanged. Setting this field to nil does not unset a memory
	// value present in the PodTemplate; to remove it, edit the PodTemplate's
	// container resources. Setting this field to the literal "0" sets the override
	// to a zero quantity and does not clear the PodTemplate's memory.
	Memory *resource.Quantity `json:"memory,omitempty"`
}
