// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type ObjectSelector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s ObjectSelector) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      s.Name,
		Namespace: s.Namespace,
	}
}

// KibanaElasticsearchAssociationSpec defines the desired state of KibanaElasticsearchAssociation
type KibanaElasticsearchAssociationSpec struct {
	Elasticsearch ObjectSelector `json:"elasticsearch"`
	Kibana        ObjectSelector `json:"kibana"`
	Monitoring    ObjectSelector `json:"monitoring,omitempty"`
}

type AssociationStatus string

const (
	AssociationPending     AssociationStatus = "Pending"
	AssociationEstablished AssociationStatus = "Established"
	AssociationFailed      AssociationStatus = "Failed"
)

// KibanaElasticsearchAssociationStatus defines the observed state of KibanaElasticsearchAssociation
type KibanaElasticsearchAssociationStatus struct {
	AssociationStatus AssociationStatus `json:"associationStatus"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KibanaElasticsearchAssociation is the Schema for the kibanaelasticsearchassociations API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:categories=elastic
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.associationStatus"
type KibanaElasticsearchAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KibanaElasticsearchAssociationSpec   `json:"spec,omitempty"`
	Status KibanaElasticsearchAssociationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KibanaElasticsearchAssociationList contains a list of KibanaElasticsearchAssociation
type KibanaElasticsearchAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KibanaElasticsearchAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KibanaElasticsearchAssociation{}, &KibanaElasticsearchAssociationList{})
}
