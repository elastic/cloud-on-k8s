// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ElasticsearchConfigSpec struct {
	Operations []ElasticsearchConfigOperation `json:"operations,omitempty"`

	// ElasticsearchRef is a reference to the Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (eg. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type ElasticsearchConfigOperation struct {
	URL  string `json:"url"`
	Body string `json:"body,omitempty"`
}

// ElasticsearchConfigStatus defines the observed state of ElasticsearchConfig
type ElasticsearchConfigStatus struct {
	// Association is the status of any auto-linking to Elasticsearch clusters.
	Association commonv1.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsMarkedForDeletion returns true if the ElasticsearchConfig is going to be deleted
func (esc *ElasticsearchConfig) IsMarkedForDeletion() bool {
	return !esc.DeletionTimestamp.IsZero()
}

func (esc *ElasticsearchConfig) ServiceAccountName() string {
	return esc.Spec.ServiceAccountName
}

func (esc *ElasticsearchConfig) Associated() commonv1.Associated {
	if esc != nil {
		return esc
	}
	return &ElasticsearchConfig{}
}

func (esc *ElasticsearchConfig) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName
}

func (esc *ElasticsearchConfig) AssociatedType() string {
	return commonv1.ElasticsearchAssociationType
}

func (esc *ElasticsearchConfig) AssociationRef() commonv1.ObjectSelector {
	return esc.Spec.ElasticsearchRef.WithDefaultNamespace(esc.Namespace)
}

func (esc *ElasticsearchConfig) AssociationConf() *commonv1.AssociationConf {
	return esc.assocConf
}

func (esc *ElasticsearchConfig) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	esc.assocConf = assocConf
}

func (esc *ElasticsearchConfig) AssociationStatus() commonv1.AssociationStatus {
	return esc.Status.Association
}

func (esc *ElasticsearchConfig) SetAssociationStatus(status commonv1.AssociationStatus) {
	esc.Status.Association = status
}

func (esc *ElasticsearchConfig) RequiresAssociation() bool {
	return esc.Spec.ElasticsearchRef.Name != ""
}

func (esc *ElasticsearchConfig) GetAssociations() []commonv1.Association {
	return []commonv1.Association{esc}
}

var _ commonv1.Associated = &ElasticsearchConfig{}
var _ commonv1.Association = &ElasticsearchConfig{}

// +kubebuilder:object:root=true

// ElasticsearchConfig is a Kubernetes CRD to represent ElasticsearchConfig
// +kubebuilder:resource:categories=elastic,shortName=esconf
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type ElasticsearchConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      ElasticsearchConfigSpec   `json:"spec,omitempty"`
	Status    ElasticsearchConfigStatus `json:"status,omitempty"`
	assocConf *commonv1.AssociationConf `json:"-"` //nolint:govet
}

// +kubebuilder:object:root=true

// ElasticsearchConfigList contains a list of ElasticsearchConfig
type ElasticsearchConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElasticsearchConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElasticsearchConfig{}, &ElasticsearchConfigList{})
}
