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
// +kubebuilder:resource:categories=elastic,shortName=aop
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.readyCount",description="Ready resources"
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

	// NamespaceSelector is a namespace selector for the resources to be configured.
	// Any Elasticsearch instances that belong to the selected namespaces will be configured to send data to AutoOps.
	// +optional
	// +kubebuilder:validation:Optional
	NamespaceSelector metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// AutoOpsRef defines a reference to a secret containing connection details for AutoOps via Cloud Connect.
	AutoOpsRef AutoOpsRef `json:"autoOpsRef,omitempty"`

	// Image is the AutoOps Agent Docker image to deploy.
	Image string `json:"image,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Agent pods
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// ServiceAccountName is used to check access to Elasticsearch resources in different namespaces.
	// Can only be used if ECK is enforcing RBAC on references (--enforce-rbac-on-refs flag).
	// The service account must have "get" permission on elasticsearch.k8s.elastic.co/elasticsearches
	// in the target namespaces.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// AutoOpsRef defines a reference to a secret containing connection details for AutoOps via Cloud Connect.
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
	// Skipped is the number of resources that are skipped from monitoring due to rbac permissions.
	Skipped int `json:"skipped,omitempty"`
	// ReadyCount is a human readable string of ready monitored resources vs all monitored resources, Ready/Resources.
	ReadyCount string `json:"readyCount,omitempty"`

	// Phase is the phase of the AutoOpsAgentPolicy.
	Phase PolicyPhase `json:"phase,omitempty"`
	// ObservedGeneration is the most recent generation observed for this AutoOpsAgentPolicy.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Details contains lightweight per-resource details.
	Details map[string]AutoOpsResourceStatus `json:"details,omitempty"`
}

// AutoOpsResourceStatus represents the status of an individual Elasticsearch resource
// monitored by an AutoOpsAgentPolicy. It is used in the Details map of AutoOpsAgentPolicyStatus
// to provide lightweight per-resource status information. Only resources that are not in a
// ready state are included in the Details map; ready resources, and resources that were skipped by resource selector and namespace selector,
// are omitted to reduce status size.
type AutoOpsResourceStatus struct {
	// Phase indicates the current state of the monitored resource.
	// Possible values are "Error" (resource encountered an error) or "Skipped"
	// (resource was skipped due to RBAC permissions).
	Phase ResourcePhase `json:"phase"`
	// Message provides a human-readable explanation of the current phase.
	// Only populated for non-ready states to provide context about why
	// the resource is not ready.
	Message string `json:"message,omitempty"`
	// Error contains the error message when the resource is in an error state.
	// Only populated when Phase is "Error".
	Error string `json:"error,omitempty"`
}

// PolicyPhase represents the current lifecycle phase of an AutoOpsAgentPolicy.
type PolicyPhase string

const (
	// ReadyPhase indicates that all monitored resources are configured and operating correctly and AutoOps agent resources are deployed correctly.
	ReadyPhase PolicyPhase = "Ready"
	// ApplyingChangesPhase indicates that configuration changes are currently being applied to AutoOps agent resources.
	ApplyingChangesPhase PolicyPhase = "ApplyingChanges"
	// InvalidPhase indicates that the AutoOpsAgentPolicy specification is invalid and cannot be processed.
	InvalidPhase PolicyPhase = "Invalid"
	// NoMonitoredResourcesPhase indicates that no Elasticsearch resources match the configured resource selector and (the optional) namespace selector.
	NoMonitoredResourcesPhase PolicyPhase = "NoMonitoredResources"
	// MonitoredResourcesNotReadyPhase indicates that one or more monitored Elasticsearch resources are not in a ready state.
	MonitoredResourcesNotReadyPhase PolicyPhase = "MonitoredResourcesNotReady"
	// AutoOpsAgentsNotReadyPhase indicates that the AutoOps agent resources are not ready.
	AutoOpsAgentsNotReadyPhase PolicyPhase = "AutoOpsAgentsNotReady"
	// ErrorPhase indicates that an error occurred while reconciling the AutoOpsAgentPolicy.
	ErrorPhase PolicyPhase = "Error"
)

// NeedsRequeue returns whether the phase requires a requeue.
func (p PolicyPhase) NeedsRequeue() bool {
	switch p {
	case ApplyingChangesPhase, MonitoredResourcesNotReadyPhase, AutoOpsAgentsNotReadyPhase, ErrorPhase:
		return true
	default:
		return false
	}
}

func (p PolicyPhase) Priority() int {
	switch p {
	case ApplyingChangesPhase, ReadyPhase:
		return 1
	case MonitoredResourcesNotReadyPhase, AutoOpsAgentsNotReadyPhase:
		return 2
	case ErrorPhase:
		return 3
	case NoMonitoredResourcesPhase:
		return 4
	case InvalidPhase:
		return 5 // Terminal - never changes
	default:
		return 0 // Unknown phases have lowest priority
	}
}

// IsMarkedForDeletion returns true if the AutoOpsAgentPolicy resource is going to be deleted.
func (p *AutoOpsAgentPolicy) IsMarkedForDeletion() bool {
	return !p.DeletionTimestamp.IsZero()
}

// ResourcePhase represents the current state of an individual Elasticsearch resource
// monitored by an AutoOpsAgentPolicy. It is used in [AutoOpsResourceStatus] to indicate
// why a resource appears in the Details map.
type ResourcePhase string

const (
	// ErrorResourcePhase indicates that the resource encountered an error during monitoring setup or operation.
	ErrorResourcePhase ResourcePhase = "Error"
	// SkippedResourcePhase indicates that the resource was skipped due to insufficient RBAC permissions.
	SkippedResourcePhase ResourcePhase = "Skipped"
)
