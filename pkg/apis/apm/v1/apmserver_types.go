// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const ApmServerContainerName = "apm-server"

// ApmServerSpec holds the specification of an APM Server.
type ApmServerSpec struct {
	// Version of the APM Server.
	Version string `json:"version,omitempty"`

	// Image is the APM Server Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of APM Server instances to deploy.
	Count int32 `json:"count,omitempty"`

	// Config holds the APM Server configuration. See: https://www.elastic.co/guide/en/apm/server/current/configuring-howto-apm-server.html
	Config *commonv1.Config `json:"config,omitempty"`

	// HTTP holds the HTTP layer configuration for the APM Server resource.
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// ElasticsearchRef is a reference to the output Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the APM Server pods.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for APM Server.
	// See: https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-apm-server.html#k8s-apm-secure-settings
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use to check access to objects in different namespaces.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
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
	commonv1.ReconcilerStatus `json:",inline"`
	Health                    ApmServerHealth `json:"health,omitempty"`
	// ExternalService is the name of the service the agents should connect to.
	ExternalService string `json:"service,omitempty"`
	// SecretTokenSecretName is the name of the Secret that contains the secret token
	SecretTokenSecretName string `json:"secretTokenSecret,omitempty"`
	// Association is the status of any auto-linking to Elasticsearch clusters.
	Association commonv1.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (as ApmServerStatus) IsDegraded(prev ApmServerStatus) bool {
	return prev.Health == ApmServerGreen && as.Health != ApmServerGreen
}

// +kubebuilder:object:root=true

// ApmServer represents an APM Server resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=apm
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="APM version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type ApmServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      ApmServerSpec             `json:"spec,omitempty"`
	Status    ApmServerStatus           `json:"status,omitempty"`
	assocConf *commonv1.AssociationConf `json:"-"` //nolint:govet
}

// +kubebuilder:object:root=true

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

func (as *ApmServer) ElasticsearchRef() commonv1.ObjectSelector {
	return as.Spec.ElasticsearchRef
}

func (as *ApmServer) SecureSettings() []commonv1.SecretSource {
	return as.Spec.SecureSettings
}

func (as *ApmServer) AssociationConf() *commonv1.AssociationConf {
	return as.assocConf
}

func (as *ApmServer) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	as.assocConf = assocConf
}
