// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApmServerSpec defines the desired state of ApmServer
type ApmServerSpec struct {
	// Version represents the version of the APM Server
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Apm Server deployment must have.
	NodeCount int32 `json:"nodeCount,omitempty"`

	// HTTP contains settings for HTTP.
	HTTP commonv1alpha1.HTTPConfig `json:"http,omitempty"`

	// +optional
	Output Output `json:"output,omitempty"`

	// PodTemplate can be used to propagate configuration to APM pods.
	// So far, only labels, Affinity and `Containers["apm"].Resources.Limits` are applied.
	// +optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// FeatureFlags are apm-specific flags that enable or disable specific experimental features
	FeatureFlags commonv1alpha1.FeatureFlags `json:"featureFlags,omitempty"`
}

// Output contains output configuration for supported outputs
type Output struct {
	// Elasticsearch configures the Elasticsearch output
	// +optional
	Elasticsearch ElasticsearchOutput `json:"elasticsearch,omitempty"`
}

// Elasticsearch contains configuration for the Elasticsearch output
type ElasticsearchOutput struct {
	// Hosts are the URLs of the output Elasticsearch nodes.
	Hosts []string `json:"hosts,omitempty"`

	// Auth configures authentication for APM Server to use.
	Auth ElasticsearchAuth `json:"auth,omitempty"`

	// SSL configures TLS-related configuration for Elasticsearch
	SSL ElasticsearchOutputSSL `json:"ssl,omitempty"`
}

// ElasticsearchOutputSSL contains TLS-related configuration for Elasticsearch
type ElasticsearchOutputSSL struct {
	// CertificateAuthoritiesSecret names a secret that contains a CA file entry to use.
	CertificateAuthoritiesSecret *string `json:"certificateAuthoritiesSecret,omitempty"`
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
}

// IsDegraded returns true if the current status is worse than the previous.
func (as ApmServerStatus) IsDegraded(prev ApmServerStatus) bool {
	return prev.Health == ApmServerGreen && as.Health != ApmServerGreen
}

// IsConfigured returns true if the output configuration is populated with non-default values.
func (e ElasticsearchOutput) IsConfigured() bool {
	return len(e.Hosts) > 0
}

// ElasticsearchAuth contains auth config for APM Server to use with an Elasticsearch cluster
// TODO: this is a good candidate for sharing/reuse between this and Kibana due to association reuse potential.
type ElasticsearchAuth struct {
	// Inline is auth provided as plaintext inline credentials.
	Inline *ElasticsearchInlineAuth `json:"inline,omitempty"`
	// SecretKeyRef is a secret that contains the credentials to use.
	SecretKeyRef *v1.SecretKeySelector `json:"secret,omitempty"`
}

// ElasticsearchInlineAuth is a basic username/password combination.
type ElasticsearchInlineAuth struct {
	// User is the username to use.
	Username string `json:"username,omitempty"`
	// Password is the password to use.
	Password string `json:"password,omitempty"`
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
