// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "CloudConnectedMode"

	unknownVersion = 0
)

func init() {
	SchemeBuilder.Register(&CloudConnectedMode{}, &CloudConnectedModeList{})
}

// +kubebuilder:object:root=true

// CloudConnectedMode represents a CloudConnectedMode resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=ccm
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.readyCount",description="Resources configured"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type CloudConnectedMode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudConnectedModeSpec   `json:"spec,omitempty"`
	Status CloudConnectedModeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudConnectedModeList contains a list of CloudConnectedMode resources.
type CloudConnectedModeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudConnectedMode `json:"items"`
}

type CloudConnectedModeSpec struct {
	ResourceSelector metav1.LabelSelector `json:"resourceSelector,omitempty"`
	// Deprecated: SecureSettings only applies to Elasticsearch and is deprecated. It must be set per application instead.
	// SecureSettings []commonv1.SecretSource       `json:"secureSettings,omitempty"`
	// Elasticsearch  ElasticsearchConfigPolicySpec `json:"elasticsearch,omitempty"`
	// Kibana         KibanaConfigPolicySpec        `json:"kibana,omitempty"`
}

type ResourceType string

const (
	ElasticsearchResourceType ResourceType = eslabel.Type
	// KibanaResourceType        ResourceType = kblabel.Type
)

type CloudConnectedModeStatus struct {
	// Resources is the number of resources to be configured.
	Resources int `json:"resources,omitempty"`
	// Ready is the number of resources successfully configured.
	Ready int `json:"ready,omitempty"`
	// Errors is the number of resources which have an incorrect configuration
	Errors int `json:"errors,omitempty"`
	// ReadyCount is a human representation of the number of resources successfully configured.
	ReadyCount string `json:"readyCount,omitempty"`
	// ObservedGeneration is the most recent generation observed for this CloudConnectedMode.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func NewStatus(scp CloudConnectedMode) CloudConnectedModeStatus {
	status := CloudConnectedModeStatus{
		// Details:            map[ResourceType]map[string]ResourcePolicyStatus{},
		// Phase:              ReadyPhase,
		ObservedGeneration: scp.Generation,
	}
	// status.setReadyCount()
	return status
}

// Update updates the policy status from its resources statuses.
func (s *CloudConnectedModeStatus) Update() {
}

// IsDegraded returns true when the CloudConnectedModeStatus resource is degraded compared to the previous status.
// func (s CloudConnectedModeStatus) IsDegraded(prev CloudConnectedModeStatus) bool {
// 	return prev.Phase == ReadyPhase && s.Phase != ReadyPhase && s.Phase != ApplyingChangesPhase
// }

// IsMarkedForDeletion returns true if the CloudConnectedMode resource is going to be deleted.
func (p *CloudConnectedMode) IsMarkedForDeletion() bool {
	return !p.DeletionTimestamp.IsZero()
}

func (s CloudConnectedModeStatus) getResourceStatusKey(nsn types.NamespacedName) string {
	return nsn.String()
}
