// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	elasticsearchv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kibanav1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StackSpec defines the desired state of a Stack
type StackSpec struct {
	// Version represents the version of the stack
	Version string `json:"version,omitempty"`

	// FeatureFlags are stack-specific flags that enable or disable specific experimental features
	FeatureFlags commonv1alpha1.FeatureFlags `json:"featureFlags,omitempty"`

	// TODO the new deployments API in EC(E) supports sequences of
	// Kibanas and Elasticsearch clusters per stack deployment

	// Elasticsearch specific configuration for the stack.
	Elasticsearch elasticsearchv1alpha1.ElasticsearchSpec `json:"elasticsearch,omitempty"`

	// Kibana spec for this stack
	Kibana kibanav1alpha1.KibanaSpec `json:"kibana,omitempty"`
}

// StackStatus defines the observed state of a Stack
type StackStatus struct {
	Elasticsearch elasticsearchv1alpha1.ElasticsearchStatus `json:"elasticsearch,omitempty"`
	Kibana        kibanav1alpha1.KibanaStatus               `json:"kibana,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Stack is the Schema for the stacks API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:categories=elastic
// +kubebuilder:printcolumn:name="es",type="string",JSONPath=".status.elasticsearch.health",description="Elasticsearch health"
// +kubebuilder:printcolumn:name="kibana",type="string",JSONPath=".status.kibana.health",description="Kibana health"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type Stack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StackSpec   `json:"spec,omitempty"`
	Status StackStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StackList contains a list of Elasticsearch
type StackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Stack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Stack{}, &StackList{})
}
