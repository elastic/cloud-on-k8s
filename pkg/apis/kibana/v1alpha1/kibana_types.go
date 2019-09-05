// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	// "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	ifs "github.com/elastic/cloud-on-k8s/pkg/controller/common/interfaces"
	
)

const (
	KibanaContainerName = "kibana"
	Kind                = "Kibana"
)

// KibanaSpec defines the desired state of Kibana
type KibanaSpec struct {
	// Version represents the version of Kibana
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Kibana deployment must have.
	NodeCount int32 `json:"nodeCount,omitempty"`

	// ElasticsearchRef references an Elasticsearch resource in the Kubernetes cluster.
	// If the namespace is not specified, the current resource namespace will be used.
	ElasticsearchRef commonv1alpha1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Elasticsearch configures how Kibana connects to Elasticsearch
	// +kubebuilder:validation:Optional
	Elasticsearch BackendElasticsearch `json:"elasticsearch,omitempty"`

	// Config represents Kibana configuration.
	Config *commonv1alpha1.Config `json:"config,omitempty"`

	// HTTP contains settings for HTTP.
	HTTP commonv1alpha1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate can be used to propagate configuration to Kibana pods.
	// This allows specifying custom annotations, labels, environment variables,
	// affinity, resources, etc. for the pods created from this NodeSpec.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings references secrets containing secure settings, to be injected
	// into Kibana keystore on each node.
	// Each individual key/value entry in the referenced secrets is considered as an
	// individual secure setting to be injected.
	// You can use the `entries` and `key` fields to consider only a subset of the secret
	// entries and the `path` field to change the target path of a secret entry key.
	// The secret must exist in the same namespace as the Kibana resource.
	SecureSettings []commonv1alpha1.SecretSource `json:"secureSettings,omitempty"`
}

// BackendElasticsearch contains configuration for an Elasticsearch backend for Kibana
type BackendElasticsearch struct {
	// ElasticsearchURL is the URL to the target Elasticsearch
	URL string `json:"url"`

	// Auth configures authentication for Kibana to use.
	Auth commonv1alpha1.ElasticsearchAuth `json:"auth,omitempty"`

	// CertificateAuthorities names a secret that contains a CA file entry to use.
	CertificateAuthorities commonv1alpha1.SecretRef `json:"certificateAuthorities,omitempty"`
}

// IsConfigured returns true if the backend configuration is populated with non-default values.
func (b BackendElasticsearch) IsConfigured() bool {
	return b.URL != "" && b.Auth.IsConfigured() && b.CertificateAuthorities.SecretName != ""
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
	commonv1alpha1.ReconcilerStatus
	Health            KibanaHealth                     `json:"health,omitempty"`
	AssociationStatus ifs.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (ks KibanaStatus) IsDegraded(prev KibanaStatus) bool {
	return prev.Health == KibanaGreen && ks.Health != KibanaGreen
}

// IsMarkedForDeletion returns true if the Kibana is going to be deleted
func (k Kibana) IsMarkedForDeletion() bool {
	return !k.DeletionTimestamp.IsZero()
}

func (k *Kibana) ElasticsearchAuth() commonv1alpha1.ElasticsearchAuth {
	return k.Spec.Elasticsearch.Auth
}

func (k *Kibana) ElasticsearchRef() commonv1alpha1.ObjectSelector {
	return k.Spec.ElasticsearchRef
}

func (k *Kibana) SecureSettings() []commonv1alpha1.SecretSource {
	return k.Spec.SecureSettings
}

// Kind can technically be retrieved from metav1.Object, but there is a bug preventing us to retrieve it
// see https://github.com/kubernetes-sigs/controller-runtime/issues/406
func (k *Kibana) Kind() string {
	return Kind
}

// +kubebuilder:object:root=true

// Kibana is the Schema for the kibanas API
// +kubebuilder:categories=elastic
// +kubebuilder:resource:shortName=kb
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Kibana version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type Kibana struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KibanaSpec   `json:"spec,omitempty"`
	Status KibanaStatus `json:"status,omitempty"`
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
