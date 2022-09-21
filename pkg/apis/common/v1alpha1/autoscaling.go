// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

var (
	// defaultMemoryRequestsToLimitsRatio is the default ratio used to convert a memory request to a memory limit in the
	// Pod resources specification. By default, we want to have the same value for both the memory request and the memory
	// limit.
	defaultMemoryRequestsToLimitsRatio = 1.0

	// defaultCPURequestsToLimitsRatio is the default ratio used to convert a CPU request to a CPU limit in the Pod
	// resources specification. By default, we want to have the same value for both the CPU request and the CPU limit.
	defaultCPURequestsToLimitsRatio = 1.0

	// DefaultPollingPeriod is the default period between 2 Elasticsearch autoscaling API polls.
	DefaultPollingPeriod = 60 * time.Second
)

// -- Elasticsearch Autoscaling API structures

// DeciderSettings allow the user to tweak autoscaling deciders.
// The map data structure complies with the <key,value> format expected by Elasticsearch.
// +kubebuilder:object:generate=false
type DeciderSettings map[string]string

// AutoscalingPolicy models the Elasticsearch autoscaling API.
type AutoscalingPolicy struct {
	// An autoscaling policy must target a unique set of roles.
	Roles []string `json:"roles,omitempty"`
	// Deciders allow the user to override default settings for autoscaling deciders.
	Deciders map[string]DeciderSettings `json:"deciders,omitempty"`
}

// -- Elastic Cloud on K8S specific structures

// +kubebuilder:object:generate=false
type AutoscalingResource interface {
	GetAutoscalingPolicySpecs() (AutoscalingPolicySpecs, error)
	GetPollingPeriod() (*metav1.Duration, error)
	GetElasticsearchAutoscalerStatus() (ElasticsearchAutoscalerStatus, error)
}

type AutoscalingPolicySpecs []AutoscalingPolicySpec

// NamedAutoscalingPolicy models an autoscaling policy as expected by the Elasticsearch policy API.
// It is identified by a unique name provided by the user.
type NamedAutoscalingPolicy struct {
	// Name identifies the autoscaling policy in the autoscaling specification.
	Name string `json:"name,omitempty"`
	// AutoscalingPolicy is the autoscaling policy as expected by the Elasticsearch API.
	AutoscalingPolicy `json:",inline"`
}

// AutoscalingPolicySpec holds a named autoscaling policy and the associated resources limits (cpu, memory, storage).
type AutoscalingPolicySpec struct {
	NamedAutoscalingPolicy `json:",inline"`

	AutoscalingResources `json:"resources"`
}

// AutoscalingResources model the limits, submitted by the user, for the supported resources in an autoscaling policy.
// Only the node count range is mandatory. For other resources, a limit range is required only
// if the Elasticsearch autoscaling capacity API returns a requirement for a given resource.
// For example, the memory limit range is only required if the autoscaling API response contains a memory requirement.
// If there is no limit range for a resource, and if that resource is not mandatory, then the resources in the NodeSets
// managed by the autoscaling policy are left untouched.
type AutoscalingResources struct {
	CPURange     *QuantityRange `json:"cpu,omitempty"`
	MemoryRange  *QuantityRange `json:"memory,omitempty"`
	StorageRange *QuantityRange `json:"storage,omitempty"`

	// NodeCountRange is used to model the minimum and the maximum number of nodes over all the NodeSets managed by the same autoscaling policy.
	NodeCountRange CountRange `json:"nodeCount"`
}

// QuantityRange models a resource limit range for resources which can be expressed with resource.Quantity.
type QuantityRange struct {
	// Min represents the lower limit for the resources managed by the autoscaler.
	Min resource.Quantity `json:"min"`
	// Max represents the upper limit for the resources managed by the autoscaler.
	Max resource.Quantity `json:"max"`
	// RequestsToLimitsRatio allows to customize Kubernetes resource Limit based on the Request.
	RequestsToLimitsRatio *resource.Quantity `json:"requestsToLimitsRatio,omitempty"`
}

// Enforce adjusts a proposed quantity to ensure it is within the quantity range.
func (qr *QuantityRange) Enforce(proposed resource.Quantity) resource.Quantity {
	if qr == nil {
		return proposed.DeepCopy()
	}
	if qr.Min.Cmp(proposed) > 0 {
		return qr.Min.DeepCopy()
	}
	if qr.Max.Cmp(proposed) < 0 {
		return qr.Max.DeepCopy()
	}
	return proposed.DeepCopy()
}

// MemoryRequestsToLimitsRatio returns the ratio between the memory request, computed by the autoscaling algorithm, and
// the limits. If no ratio has been specified by the user then a default value is returned.
func (ar AutoscalingResources) MemoryRequestsToLimitsRatio() float64 {
	if ar.MemoryRange == nil || ar.MemoryRange.RequestsToLimitsRatio == nil {
		return defaultMemoryRequestsToLimitsRatio
	}
	return ar.MemoryRange.RequestsToLimitsRatio.AsApproximateFloat64()
}

// CPURequestsToLimitsRatio returns the ratio between the CPU request, computed by the autoscaling algorithm, and
// the limits. If no ratio has been specified by the user then a default value is returned.
func (ar AutoscalingResources) CPURequestsToLimitsRatio() float64 {
	if ar.CPURange == nil || ar.CPURange.RequestsToLimitsRatio == nil {
		return defaultCPURequestsToLimitsRatio
	}
	return ar.CPURange.RequestsToLimitsRatio.AsApproximateFloat64()
}

type CountRange struct {
	// Min represents the minimum number of nodes in a tier.
	Min int32 `json:"min"`
	// Max represents the maximum number of nodes in a tier.
	Max int32 `json:"max"`
}

// Enforce adjusts a node count to ensure that it is within the range.
func (cr *CountRange) Enforce(count int32) int32 {
	if count < cr.Min {
		return cr.Min
	} else if count > cr.Max {
		return cr.Max
	}
	return count
}

// IsMemoryDefined returns true if the user specified memory limits.
func (aps AutoscalingPolicySpec) IsMemoryDefined() bool {
	return aps.MemoryRange != nil
}

// IsCPUDefined returns true if the user specified cpu limits.
func (aps AutoscalingPolicySpec) IsCPUDefined() bool {
	return aps.CPURange != nil
}

// IsStorageDefined returns true if the user specified storage limits.
func (aps AutoscalingPolicySpec) IsStorageDefined() bool {
	return aps.StorageRange != nil
}

// FindByRoles returns the autoscaling specification associated with a set of roles or nil if not found.
func (aps AutoscalingPolicySpecs) FindByRoles(roles []string) *AutoscalingPolicySpec {
	for _, autoscalingPolicySpec := range aps {
		if !rolesMatch(autoscalingPolicySpec.Roles, roles) {
			continue
		}
		return &autoscalingPolicySpec
	}
	return nil
}

// rolesMatch compares two set of roles and returns true if both sets contain the exact same roles.
func rolesMatch(roles1, roles2 []string) bool {
	if len(roles1) != len(roles2) {
		return false
	}
	rolesInRoles1 := set.Make(roles1...)
	for _, roleInRoles2 := range roles2 {
		if !rolesInRoles1.Has(roleInRoles2) {
			return false
		}
	}
	return true
}

// AutoscalingPoliciesByRole returns the names of the autoscaling policies indexed by roles.
func (aps AutoscalingPolicySpecs) AutoscalingPoliciesByRole() map[string][]string {
	policiesByRole := make(map[string][]string)
	for _, policySpec := range aps {
		for _, role := range policySpec.Roles {
			policiesByRole[role] = append(policiesByRole[role], policySpec.Name)
		}
	}
	return policiesByRole
}
