// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	APMServerContainerName = "apm-server"
	Kind                   = "ApmServer"
)

// ApmServerSpec defines the desired state of ApmServer
type ApmServerSpec struct {
	// Version represents the version of the APM Server
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Apm Server deployment must have.
	NodeCount int32 `json:"nodeCount,omitempty"`

	// Config represents the APM configuration.
	Config *commonv1alpha1.Config `json:"config,omitempty"`

	// HTTP contains settings for HTTP.
	HTTP commonv1alpha1.HTTPConfig `json:"http,omitempty"`

	// ElasticsearchRef references an Elasticsearch resource in the Kubernetes cluster.
	// If the namespace is not specified, the current resource namespace will be used.
	ElasticsearchRef commonv1alpha1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Elasticsearch configures how the APM server connects to Elasticsearch
	// +optional
	Elasticsearch ElasticsearchOutput `json:"elasticsearch,omitempty"`

	// PodTemplate can be used to propagate configuration to APM Server pods.
	// This allows specifying custom annotations, labels, environment variables,
	// affinity, resources, etc. for the pods created from this NodeSpec.
	// +optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings references secrets containing secure settings, to be injected
	// into the APM keystore on each node.
	// Each individual key/value entry in the referenced secrets is considered as an
	// individual secure setting to be injected.
	// You can use the `items` and `key` fields to consider only a subset of the secret
	// entries and the `path` field to change the target path of a secret entry key.
	// The secret must exist in the same namespace as the APM resource.
	SecureSettings []corev1.SecretVolumeSource `json:"secureSettings,omitempty"`

	// FeatureFlags are apm-specific flags that enable or disable specific experimental features
	FeatureFlags commonv1alpha1.FeatureFlags `json:"featureFlags,omitempty"`
}

// Elasticsearch contains configuration for the Elasticsearch output
type ElasticsearchOutput struct {

	// Hosts are the URLs of the output Elasticsearch nodes.
	Hosts []string `json:"hosts,omitempty"`

	// Auth configures authentication for APM Server to use.
	Auth commonv1alpha1.ElasticsearchAuth `json:"auth,omitempty"`

	// SSL configures TLS-related configuration for Elasticsearch
	SSL ElasticsearchOutputSSL `json:"ssl,omitempty"`
}

// ElasticsearchOutputSSL contains TLS-related configuration for Elasticsearch
type ElasticsearchOutputSSL struct {
	// CertificateAuthorities is a secret that contains a `tls.crt` entry that contain certificates for server
	// verifications.
	CertificateAuthorities commonv1alpha1.SecretRef `json:"certificateAuthorities,omitempty"`
}

// ApmServerHealth expresses the status of the Apm Server instances.
type ApmServerHealth string

const (
	// ApmServerRed means no instance is currently available.
	ApmServerRed ApmServerHealth = "red"
	// ApmServerGreen means at least one instance is available.
	ApmServerGreen ApmServerHealth = "green"
)

// ApmServerStatus defines the observed state of ApmServer
type ApmServerStatus struct {
	commonv1alpha1.ReconcilerStatus
	Health ApmServerHealth `json:"health,omitempty"`
	// ExternalService is the name of the service the agents should connect to.
	ExternalService string `json:"service,omitempty"`
	// SecretTokenSecretName is the name of the Secret that contains the secret token
	SecretTokenSecretName string `json:"secretTokenSecret,omitempty"`
	// Association is the status of any auto-linking to Elasticsearch clusters.
	Association commonv1alpha1.AssociationStatus
}

// IsDegraded returns true if the current status is worse than the previous.
func (as ApmServerStatus) IsDegraded(prev ApmServerStatus) bool {
	return prev.Health == ApmServerGreen && as.Health != ApmServerGreen
}

// IsConfigured returns true if the output configuration is populated with non-default values.
func (e ElasticsearchOutput) IsConfigured() bool {
	return len(e.Hosts) > 0
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApmServer is the Schema for the apmservers API
// +k8s:openapi-gen=true
// +kubebuilder:categories=elastic
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="APM version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type ApmServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApmServerSpec   `json:"spec,omitempty"`
	Status ApmServerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApmServerList contains a list of ApmServer
type ApmServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApmServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApmServer{}, &ApmServerList{})
}

// IsMarkedForDeletion returns true if the APM is going to be deleted
func (as *ApmServer) IsMarkedForDeletion() bool {
	return !as.DeletionTimestamp.IsZero()
}

func (as *ApmServer) ElasticsearchAuth() commonv1alpha1.ElasticsearchAuth {
	return as.Spec.Elasticsearch.Auth
}

func (as *ApmServer) ElasticsearchRef() commonv1alpha1.ObjectSelector {
	return as.Spec.ElasticsearchRef
}

func (as *ApmServer) SecureSettings() []corev1.SecretVolumeSource {
	return as.Spec.SecureSettings
}

// Kind can technically be retrieved from metav1.Object, but there is a bug preventing us to retrieve it
// see https://github.com/kubernetes-sigs/controller-runtime/issues/406
func (as *ApmServer) Kind() string {
	return Kind
}
