// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "AutoOpsAgentPolicy"
)

func init() {
	SchemeBuilder.Register(&AutoOpsAgentPolicy{}, &AutoOpsAgentPolicyList{})
}

// +kubebuilder:object:root=true

// AutoOpsAgentPolicy represents an Elastic AutoOps Policy resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=autoops
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Ready resources"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type AutoOpsAgentPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoOpsAgentPolicySpec   `json:"spec,omitempty"`
	Status AutoOpsAgentPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AutoOpsAgentPolicyList contains a list of AutoOpsAgentPolicy resources.
type AutoOpsAgentPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutoOpsAgentPolicy `json:"items"`
}

type AutoOpsAgentPolicySpec struct {
	// Version of the AutoOpsAgentPolicy.
	Version string `json:"version"`
	// ResourceSelector is a label selector for the resources to be configured.
	// Any Elasticsearch instances that match the selector will be configured to send data to AutoOps.
	ResourceSelector metav1.LabelSelector `json:"resourceSelector,omitempty"`

	// AutoOpsRef defines a reference to Elastic AutoOps containing connection details for external AutoOps.
	AutoOpsRef AutoOpsRef `json:"autoOpsRef,omitempty"`

	// Image is the AutoOps Agent Docker image to deploy.
	Image string `json:"image,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Agent pods
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// AutoOpsRef defines a reference to Elastic AutoOps containing connection details for external AutoOps.
type AutoOpsRef struct {
	// SecretName references a Secret containing connection details for external AutoOps.
	// Required when connecting via Cloud Connect. The secret must contain:
	// - `cloud-connected-mode-api-key`: Cloud Connected Mode API key
	// - `autoops-otel-url`: AutoOps OpenTelemetry endpoint URL
	// - `autoops-token`: AutoOps authentication token
	// - `cloud-connected-mode-api-url`: (optional) Cloud Connected Mode API URL
	// This field cannot be used in combination with `name`.
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

type AutoOpsAgentPolicyStatus struct {
	// Resources is the number of resources that match the ResourceSelector.
	Resources int `json:"resources"`
	// Ready is the number of resources that are in a ready state.
	Ready int `json:"ready"`
	// Errors is the number of resources that are in an error state.
	Errors int `json:"errors"`
	// Phase is the phase of the AutoOpsAgentPolicy.
	Phase PolicyPhase `json:"phase,omitempty"`
	// ObservedGeneration is the most recent generation observed for this AutoOpsAgentPolicy.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type PolicyPhase string

const (
	ReadyPhase             PolicyPhase = "Ready"
	ApplyingChangesPhase   PolicyPhase = "ApplyingChanges"
	InvalidPhase           PolicyPhase = "Invalid"
	NoResourcesPhase       PolicyPhase = "NoResources"
	ResourcesNotReadyPhase PolicyPhase = "ResourcesNotReady"
	ErrorPhase             PolicyPhase = "Error"
)

// IsMarkedForDeletion returns true if the AutoOpsAgentPolicy resource is going to be deleted.
func (p *AutoOpsAgentPolicy) IsMarkedForDeletion() bool {
	return !p.DeletionTimestamp.IsZero()
}
