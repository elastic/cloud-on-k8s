// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "AutoOpsAgentPolicy"

	unknownVersion = 0
)

func init() {
	SchemeBuilder.Register(&AutoOpsAgentPolicy{}, &AutoOpsAgentPolicyList{})
}

// +kubebuilder:object:root=true

// AutoOpsAgentPolicy represents an AutoOpsAgentPolicy resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=autoops
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.readyCount",description="Resources configured"
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
	// ResourceSelector is a label selector for the resources to be configured.
	// Any Elasticsearch instances that match the selector will be configured to send data to AutoOps.
	ResourceSelector metav1.LabelSelector `json:"resourceSelector,omitempty"`
	// Config holds the AutoOpsAgentPolicy configuration.
	// The contents of the referenced secret requires the following format:
	//   kind: Secret
	//   apiVersion: v1
	//   metadata:
	//     name: autoops-agent-policy-config
	//   stringData:
	//     ccmApiKey: aslkfjsldkjfslkdjflksdjfl
	//     tempResourceID: u857abce4-9214-446b-951c-a1644b7d204ao
	//     autoOpsOTelURL: https://otel.auto-ops.console.qa.cld.elstc.co
	//     autoOpsToken: skdfjdskjf
	Config *commonv1.Config `json:"config,omitempty"`
	// AutoOpsRef is a reference to an AutoOps instance running in the same Kubernetes cluster.
	// (TODO) AutoOpsRef is not yet implemented.
	// AutoOpsRef commonv1.ObjectSelector `json:"autoOpsRef,omitempty"`
}

type AutoOpsAgentPolicyStatus struct {
	// Resources is the number of resources that match the ResourceSelector.
	Resources int `json:"resources,omitempty"`
	// Ready is the number of resources that are in a ready state.
	Ready int `json:"ready,omitempty"`
	// Errors is the number of resources that are in an error state.
	Errors int `json:"errors,omitempty"`
	// ObservedGeneration is the most recent generation observed for this AutoOpsAgentPolicy.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func NewStatus(policy AutoOpsAgentPolicy) AutoOpsAgentPolicyStatus {
	status := AutoOpsAgentPolicyStatus{
		// Details:            map[ResourceType]map[string]ResourcePolicyStatus{},
		// Phase:              ReadyPhase,
		ObservedGeneration: policy.Generation,
	}
	return status
}

// Update updates the policy status from its resources statuses.
func (s *AutoOpsAgentPolicyStatus) Update() {
}

// IsDegraded returns true when the AutoOpsAgentPolicyStatus resource is degraded compared to the previous status.
// func (s AutoOpsAgentPolicyStatus) IsDegraded(prev AutoOpsAgentPolicyStatus) bool {
// 	return prev.Phase == ReadyPhase && s.Phase != ReadyPhase && s.Phase != ApplyingChangesPhase
// }

// IsMarkedForDeletion returns true if the AutoOpsAgentPolicy resource is going to be deleted.
func (p *AutoOpsAgentPolicy) IsMarkedForDeletion() bool {
	return !p.DeletionTimestamp.IsZero()
}

func (s AutoOpsAgentPolicyStatus) getResourceStatusKey(nsn types.NamespacedName) string {
	return nsn.String()
}
