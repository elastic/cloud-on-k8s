// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Logstash"
)

// LogstashSpec defines the desired state of Logstash
type LogstashSpec struct {
	// Version of the Logstash.
	Version string `json:"version"`

	Count int32 `json:"count,omitempty"`

	// Image is the Logstash Docker image to deploy. Version and Type have to match the Logstash in the image.
	// +kubebuilder:validation:Optional
	Image string `json:"image,omitempty"`

	// Config holds the Logstash configuration. At most one of [`Config`, `ConfigRef`] can be specified.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains a reference to an existing Kubernetes Secret holding the Logstash configuration.
	// Logstash settings must be specified as yaml, under a single "logstash.yml" entry. At most one of [`Config`, `ConfigRef`]
	// can be specified.
	// +kubebuilder:validation:Optional
	ConfigRef *commonv1.ConfigSource `json:"configRef,omitempty"`

	// HTTP holds the HTTP layer configuration for the Logstash Metrics API
	// TODO: This should likely be changed to a more general `Services LogstashService[]`, where `LogstashService` looks
	//       a lot like `HTTPConfig`, but is applicable for more than just an HTTP endpoint, as logstash may need to
	//       be opened up for other services: beats, TCP, UDP, etc, inputs
	// +kubebuilder:validation:Optional
	Services []LogstashService `json:"services,omitempty"`

	// PodTemplate provides customisation options for the Logstash pods.
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying StatefulSet.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Logstash.
	// Secrets data can be then referenced in the Logstash config using the Secret's keys or as specified in `Entries` field of
	// each SecureSetting.
	// +kubebuilder:validation:Optional
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type LogstashService struct {
	Name string `json:"name,omitempty"`
	// Service defines the template for the associated Kubernetes Service object.
	Service commonv1.ServiceTemplate `json:"service,omitempty"`
	// TLS defines options for configuring TLS for HTTP.
	TLS commonv1.TLSOptions `json:"tls,omitempty"`
}

// LogstashStatus defines the observed state of Logstash
type LogstashStatus struct {
	// Version of the stack resource currently running. During version upgrades, multiple versions may run
	// in parallel: this value specifies the lowest version currently running.
	Version string `json:"version,omitempty"`

	// +kubebuilder:validation:Optional
	ExpectedNodes int32 `json:"expectedNodes,omitempty"`
	// +kubebuilder:validation:Optional
	AvailableNodes int32 `json:"availableNodes,omitempty"`

	// ObservedGeneration is the most recent generation observed for this Logstash instance.
	// It corresponds to the metadata generation, which is updated on mutation by the API Server.
	// If the generation observed in status diverges from the generation in metadata, the Logstash
	// controller has not yet processed the changes contained in the Logstash specification.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Logstash is the Schema for the logstashes API
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories=elastic,shortName=ls
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="expected",type="integer",JSONPath=".status.expectedNodes",description="Expected nodes"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Logstash version"
// +kubebuilder:subresource:scale:specpath=.spec.count,statuspath=.status.count,selectorpath=.status.selector
// +kubebuilder:storageversion
type Logstash struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogstashSpec   `json:"spec,omitempty"`
	Status LogstashStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LogstashList contains a list of Logstash
type LogstashList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Logstash `json:"items"`
}

func (l *Logstash) ServiceAccountName() string {
	return l.Spec.ServiceAccountName
}

func (l *Logstash) SecureSettings() []commonv1.SecretSource {
	return l.Spec.SecureSettings
}

// IsMarkedForDeletion returns true if the Logstash is going to be deleted
func (l *Logstash) IsMarkedForDeletion() bool {
	return !l.DeletionTimestamp.IsZero()
}

// GetObservedGeneration will return the observedGeneration from the Elastic Logstash's status.
func (l *Logstash) GetObservedGeneration() int64 {
	return l.Status.ObservedGeneration
}

func init() {
	SchemeBuilder.Register(&Logstash{}, &LogstashList{})
}
