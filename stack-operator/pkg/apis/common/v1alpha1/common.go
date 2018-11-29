package v1alpha1

import "k8s.io/api/core/v1"

// ResourcesSpec defines the resources to be allocated to a pod
type ResourcesSpec struct {
	// Limits represents the limits to considerate for these resources
	Limits v1.ResourceList `json:"limits,omitempty"`
}

// ReconcilerStatus represents status information about desired/available nodes.
type ReconcilerStatus struct {
	AvailableNodes int
}
