// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	common_name "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
)

const (
	EPRContainerName = "package-registry"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "PackageRegistry"
)

// Namer is a Namer that is configured with the defaults for resources related to a Package Registry resource.
var Namer = common_name.NewNamer("epr")

// PackageRegistrySpec holds the specification of an Elastic Package Registry instance.
type PackageRegistrySpec struct {
	// Version of Elastic Package Registry.
	Version string `json:"version"`

	// Image is the Elastic Package Registry Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of Elastic Package Registry instances to deploy.
	Count int32 `json:"count,omitempty"`

	// Config holds the PackageRegistry configuration. See: https://github.com/elastic/package-registry/blob/main/config.reference.yml
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains a reference to an existing Kubernetes Secret holding the Elastic Package Registry configuration.
	// Configuration settings are merged and have precedence over settings specified in `config`.
	// +kubebuilder:validation:Optional
	ConfigRef *commonv1.ConfigSource `json:"configRef,omitempty"`

	// HTTP holds the HTTP layer configuration for Elastic Package Registry.
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Elastic Package Registry pods
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// PackageRegistryStatus defines the observed state of Elastic Package Registry
type PackageRegistryStatus struct {
	commonv1.DeploymentStatus `json:",inline"`

	// ObservedGeneration is the most recent generation observed for this Elastic Package Registry.
	// It corresponds to the metadata generation, which is updated on mutation by the API Server.
	// If the generation observed in status diverges from the generation in metadata, the Elastic Package Registry
	// controller has not yet processed the changes contained in the Elastic Package Registry specification.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true

// PackageRegistry represents an Elastic Package Registry resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=epr
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="PackageRegistry version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:scale:specpath=.spec.count,statuspath=.status.count,selectorpath=.status.selector
// +kubebuilder:storageversion
type PackageRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PackageRegistrySpec   `json:"spec,omitempty"`
	Status PackageRegistryStatus `json:"status,omitempty"`
}

// IsMarkedForDeletion returns true if the Elastic Package Registry instance is going to be deleted
func (m *PackageRegistry) IsMarkedForDeletion() bool {
	return !m.DeletionTimestamp.IsZero()
}

// GetObservedGeneration will return the observedGeneration from the Elastic Package Registry status.
func (m *PackageRegistry) GetObservedGeneration() int64 {
	return m.Status.ObservedGeneration
}

// +kubebuilder:object:root=true

// PackageRegistryList contains a list of PackageRegistry
type PackageRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PackageRegistry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PackageRegistry{}, &PackageRegistryList{})
}
