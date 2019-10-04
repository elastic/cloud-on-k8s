// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
)

const KibanaContainerName = "kibana"

// KibanaSpec defines the desired state of Kibana
type KibanaSpec struct {
	// Version represents the version of Kibana
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// Count defines how many nodes the Kibana deployment must have.
	Count int32 `json:"count,omitempty"`

	// ElasticsearchRef references an Elasticsearch resource in the Kubernetes cluster.
	// If the namespace is not specified, the current resource namespace will be used.
	ElasticsearchRef commonv1beta1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Config represents Kibana configuration.
	Config *commonv1beta1.Config `json:"config,omitempty"`

	// HTTP contains settings for HTTP.
	HTTP commonv1beta1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate can be used to propagate configuration to Kibana pods.
	// This allows specifying custom annotations, labels, environment variables,
	// affinity, resources, etc. for the pods created from this spec.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings references secrets containing secure settings, to be injected
	// into Kibana keystore on each node.
	// Each individual key/value entry in the referenced secrets is considered as an
	// individual secure setting to be injected.
	// You can use the `entries` and `key` fields to consider only a subset of the secret
	// entries and the `path` field to change the target path of a secret entry key.
	// The secret must exist in the same namespace as the Kibana resource.
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

// +kubebuilder:object:root=true

// Kibana is the Schema for the kibanas API
// +kubebuilder:resource:categories=elastic,shortName=kb
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Kibana version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Kibana struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      KibanaSpec                     `json:"spec,omitempty"`
	Status    KibanaStatus                   `json:"status,omitempty"`
	assocConf *commonv1beta1.AssociationConf `json:"-"` //nolint:govet
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
