// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ResourcesSpec defines the resources to be allocated to a pod
type ResourcesSpec struct {
	// Limits represents the limits to considerate for these resources
	Limits corev1.ResourceList `json:"limits,omitempty"`
}

// ReconcilerStatus represents status information about desired/available nodes.
type ReconcilerStatus struct {
	AvailableNodes int `json:"availableNodes,omitempty"`
}

// SecretRef reference a secret by name.
type SecretRef struct {
	SecretName string `json:"secretName"`
}

// ObjectSelector allows to specify a reference to an object across namespace boundaries.
type ObjectSelector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// NamespacedName is a convenience method to turn an ObjectSelector into a NamespaceName.
func (s ObjectSelector) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      s.Name,
		Namespace: s.Namespace,
	}
}
