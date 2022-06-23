// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1beta1"
)

const KibanaContainerName = "kibana"

// KibanaSpec holds the specification of a Kibana instance.
type KibanaSpec struct {
	// Version of Kibana.
	Version string `json:"version,omitempty"`

	// Image is the Kibana Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of Kibana instances to deploy.
	Count int32 `json:"count,omitempty"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1beta1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Config holds the Kibana configuration. See: https://www.elastic.co/guide/en/kibana/current/settings.html
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1beta1.Config `json:"config,omitempty"`

	// HTTP holds the HTTP layer configuration for Kibana.
	HTTP commonv1beta1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Kibana pods
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Kibana.
	SecureSettings []commonv1beta1.SecretSource `json:"secureSettings,omitempty"`
}

// KibanaHealth expresses the status of the Kibana instances.
type KibanaHealth string

const (
	// KibanaRed means no instance is currently available.
	KibanaRed KibanaHealth = "red"
	// KibanaGreen means at least one instance is available.
	KibanaGreen KibanaHealth = "green"
)

// KibanaStatus defines the observed state of Kibana
type KibanaStatus struct {
	commonv1beta1.ReconcilerStatus `json:",inline"`
	Health                         KibanaHealth                    `json:"health,omitempty"`
	AssociationStatus              commonv1beta1.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (ks KibanaStatus) IsDegraded(prev KibanaStatus) bool {
	return prev.Health == KibanaGreen && ks.Health != KibanaGreen
}

// IsMarkedForDeletion returns true if the Kibana is going to be deleted
func (k Kibana) IsMarkedForDeletion() bool {
	return !k.DeletionTimestamp.IsZero()
}

func (k *Kibana) ElasticsearchRef() commonv1beta1.ObjectSelector {
	return k.Spec.ElasticsearchRef
}

func (k *Kibana) SecureSettings() []commonv1beta1.SecretSource {
	return k.Spec.SecureSettings
}

func (k *Kibana) AssociationConf() *commonv1beta1.AssociationConf {
	return k.assocConf
}

func (k *Kibana) SetAssociationConf(assocConf *commonv1beta1.AssociationConf) {
	k.assocConf = assocConf
}

// RequiresAssociation returns true if the spec specifies an Elasticsearch reference.
func (k *Kibana) RequiresAssociation() bool {
	return k.Spec.ElasticsearchRef.Name != ""
}

// +kubebuilder:object:root=true

// Kibana represents a Kibana resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=kb
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Kibana version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type Kibana struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      KibanaSpec                     `json:"spec,omitempty"`
	Status    KibanaStatus                   `json:"status,omitempty"`
	assocConf *commonv1beta1.AssociationConf `json:"-"`
}

// +kubebuilder:object:root=true

// KibanaList contains a list of Kibana
type KibanaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kibana `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kibana{}, &KibanaList{})
}
