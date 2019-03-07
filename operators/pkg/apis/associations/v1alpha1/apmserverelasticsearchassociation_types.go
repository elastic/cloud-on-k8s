// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApmServerElasticsearchAssociationSpec defines the desired state of ApmServerElasticsearchAssociation
type ApmServerElasticsearchAssociationSpec struct {
	// Elasticsearch refers to the Elasticsearch resource
	Elasticsearch ObjectSelector `json:"elasticsearch"`
	// ApmServer refers to the ApmServer resource.
	ApmServer     ObjectSelector `json:"apmServer"`
}

// ApmServerElasticsearchAssociationStatus defines the observed state of ApmServerElasticsearchAssociation
type ApmServerElasticsearchAssociationStatus struct {
	// AssociationStatus indicates the current state of the association.
	AssociationStatus AssociationStatus `json:"associationStatus"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApmServerElasticsearchAssociation is the Schema for the apmserverelasticsearchassociations API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aea
// +kubebuilder:categories=elastic
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.associationStatus"
type ApmServerElasticsearchAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApmServerElasticsearchAssociationSpec   `json:"spec,omitempty"`
	Status ApmServerElasticsearchAssociationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApmServerElasticsearchAssociationList contains a list of ApmServerElasticsearchAssociation
type ApmServerElasticsearchAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApmServerElasticsearchAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApmServerElasticsearchAssociation{}, &ApmServerElasticsearchAssociationList{})
}
