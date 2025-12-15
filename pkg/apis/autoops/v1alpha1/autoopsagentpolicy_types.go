// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
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
	// Config holds the AutoOps agent configuration overrides for Elasticsearch monitoring.
	// This configuration is intended to override parts of the autoops_es.yml configmap.
	// See: https://github.com/elastic/elastic-agent/blob/c6eaa3c903d4357824d345ee2002123fdffbec91/internal/pkg/otel/samples/linux/autoops_es.yml
	//
	// This is currently unimplemented.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`
	// ConfigRef references a secret holding the AutoOpsAgentPolicy configuration overrides for Elasticsearch monitoring.
	// This configuration is intended to override parts of the autoops_es.yml configmap.
	// See: https://github.com/elastic/elastic-agent/blob/c6eaa3c903d4357824d345ee2002123fdffbec91/internal/pkg/otel/samples/linux/autoops_es.yml
	//
	// This is currently unimplemented.
	//
	ConfigRef commonv1.ConfigSource `json:"configRef,omitempty"`

	// AutoOpsRef is a reference to Elastic AutoOps either running in the same Kubernetes cluster (unimplemented) or
	// connected via (Elastic Cloud Connect)[https://www.elastic.co/docs/deploy-manage/cloud-connect].
	//
	// If using Elastic Cloud Connect the contents of the referenced secret requires the following format:
	//   kind: Secret
	//   apiVersion: v1
	//   metadata:
	//     name: autoops-agent-policy-config
	//   stringData:
	//     ccmApiKey: aslkfjsldkjfslkdjflksdjfl
	//     autoOpsOTelURL: https://otel.auto-ops.console.qa.cld.elstc.co
	//     autoOpsToken: skdfjdskjf
	AutoOpsRef commonv1.ObjectSelector `json:"autoOpsRef,omitempty"`

	// Image is the AutoOps Agent Docker image to deploy.
	Image string `json:"image,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Agent pods
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

type AutoOpsAgentPolicyStatus struct {
	// Resources is the number of resources that match the ResourceSelector.
	Resources int `json:"resources,omitempty"`
	// Ready is the number of resources that are in a ready state.
	Ready int `json:"ready,omitempty"`
	// Errors is the number of resources that are in an error state.
	Errors int `json:"errors,omitempty"`
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
