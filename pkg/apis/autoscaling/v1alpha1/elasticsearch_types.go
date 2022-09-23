// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "ElasticsearchAutoscaler"
)

// +kubebuilder:object:root=true

// ElasticsearchAutoscaler represents an ElasticsearchAutoscaler resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=esa
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.elasticsearchRef.name"
// +kubebuilder:printcolumn:name="Active",type="string",JSONPath=".status.conditions[?(@.type=='Active')].status"
// +kubebuilder:printcolumn:name="Healthy",type="string",JSONPath=".status.conditions[?(@.type=='Healthy')].status"
// +kubebuilder:printcolumn:name="Limited",type="string",JSONPath=".status.conditions[?(@.type=='Limited')].status"
type ElasticsearchAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticsearchAutoscalerSpec            `json:"spec,omitempty"`
	Status v1alpha1.ElasticsearchAutoscalerStatus `json:"status,omitempty"`
}

var _ v1alpha1.AutoscalingResource = &ElasticsearchAutoscaler{}

// ElasticsearchAutoscalerSpec holds the specification of an Elasticsearch autoscaler resource.
type ElasticsearchAutoscalerSpec struct {
	// +kubebuilder:validation:Required
	ElasticsearchRef ElasticsearchRef `json:"elasticsearchRef,omitempty"`

	AutoscalingPolicySpecs v1alpha1.AutoscalingPolicySpecs `json:"policies"`

	// +kubebuilder:validation:Optional
	// PollingPeriod is the period at which to synchronize with the Elasticsearch autoscaling API.
	PollingPeriod *metav1.Duration `json:"pollingPeriod,omitempty"`
}

func (esa *ElasticsearchAutoscaler) GetAutoscalingPolicySpecs() (v1alpha1.AutoscalingPolicySpecs, error) {
	if esa == nil {
		return nil, nil
	}
	return esa.Spec.AutoscalingPolicySpecs, nil
}

func (esa *ElasticsearchAutoscaler) GetPollingPeriod() (*metav1.Duration, error) {
	if esa == nil {
		return nil, nil
	}
	return esa.Spec.PollingPeriod, nil
}

func (esa *ElasticsearchAutoscaler) GetElasticsearchAutoscalerStatus() (v1alpha1.ElasticsearchAutoscalerStatus, error) {
	if esa == nil {
		return v1alpha1.ElasticsearchAutoscalerStatus{}, nil
	}
	return esa.Status, nil
}

// ElasticsearchRef is a reference to an Elasticsearch cluster that exists in the same namespace.
type ElasticsearchRef struct {
	// Name is the name of the Elasticsearch resource to scale automatically.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
}

// +kubebuilder:object:root=true

// ElasticsearchAutoscalerList contains a list of Elasticsearch autoscaler resources.
type ElasticsearchAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElasticsearchAutoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElasticsearchAutoscaler{}, &ElasticsearchAutoscalerList{})
}
