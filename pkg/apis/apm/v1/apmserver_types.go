// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const ApmServerContainerName = "apm-server"

// ApmServerSpec holds the specification of an APM Server.
type ApmServerSpec struct {
	// Version of the APM Server.
	Version string `json:"version"`

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

	// KibanaRef is a reference to a Kibana instance running in the same Kubernetes cluster.
	// It allows APM agent central configuration management in Kibana.
	KibanaRef commonv1.ObjectSelector `json:"kibanaRef,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the APM Server pods.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for APM Server.
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (eg. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
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
	// ElasticsearchAssociationStatus is the status of any auto-linking to Elasticsearch clusters.
	ElasticsearchAssociationStatus commonv1.AssociationStatus `json:"elasticsearchAssociationStatus,omitempty"`
	// KibanaAssociationStatus is the status of any auto-linking to Kibana.
	KibanaAssociationStatus commonv1.AssociationStatus `json:"kibanaAssociationStatus,omitempty"`
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

	Spec            ApmServerSpec             `json:"spec,omitempty"`
	Status          ApmServerStatus           `json:"status,omitempty"`
	esAssocConf     *commonv1.AssociationConf `json:"-"` //nolint:govet
	kibanaAssocConf *commonv1.AssociationConf `json:"-"` //nolint:govet
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

func (as *ApmServer) SecureSettings() []commonv1.SecretSource {
	return as.Spec.SecureSettings
}

func (as *ApmServer) ServiceAccountName() string {
	return as.Spec.ServiceAccountName
}

// EffectiveVersion returns the version reported by APM server. For development builds APM server does not use the SNAPSHOT suffix.
func (as *ApmServer) EffectiveVersion() string {
	return strings.TrimSuffix(as.Spec.Version, "-SNAPSHOT")
}

func (as *ApmServer) GetAssociations() []commonv1.Association {
	return []commonv1.Association{
		&ApmEsAssociation{ApmServer: as},
		&ApmKibanaAssociation{ApmServer: as},
	}
}

// ApmEsAssociation helps to manage the APMServer / Elasticsearch association
type ApmEsAssociation struct {
	*ApmServer
}

var _ commonv1.Association = &ApmEsAssociation{}

func NewApmEsAssociation(as *ApmServer) *ApmEsAssociation {
	return &ApmEsAssociation{ApmServer: as}
}

func (aes *ApmEsAssociation) Associated() commonv1.Associated {
	if aes == nil {
		return nil
	}
	if aes.ApmServer == nil {
		aes.ApmServer = &ApmServer{}
	}
	return aes.ApmServer
}

func (aes *ApmEsAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName
}

func (aes *ApmEsAssociation) AssociatedType() string {
	return commonv1.ElasticsearchAssociationType
}

func (aes *ApmEsAssociation) AssociationRef() commonv1.ObjectSelector {
	return aes.Spec.ElasticsearchRef.WithDefaultNamespace(aes.Namespace)
}

func (aes *ApmEsAssociation) AssociationConf() *commonv1.AssociationConf {
	return aes.esAssocConf
}

func (aes *ApmEsAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	aes.esAssocConf = assocConf
}

func (aes *ApmEsAssociation) AssociationStatus() commonv1.AssociationStatus {
	return aes.Status.ElasticsearchAssociationStatus
}

func (aes *ApmEsAssociation) SetAssociationStatus(status commonv1.AssociationStatus) {
	aes.Status.ElasticsearchAssociationStatus = status
}

var _ commonv1.Association = &ApmKibanaAssociation{}

// ApmServer / Kibana association helper
type ApmKibanaAssociation struct {
	*ApmServer
}

func NewApmKibanaAssociation(as *ApmServer) *ApmKibanaAssociation {
	return &ApmKibanaAssociation{ApmServer: as}
}

func (akb *ApmKibanaAssociation) Associated() commonv1.Associated {
	if akb == nil {
		return nil
	}
	if akb.ApmServer == nil {
		akb.ApmServer = &ApmServer{}
	}
	return akb.ApmServer
}

func (akb *ApmKibanaAssociation) AssociationConfAnnotationName() string {
	return commonv1.KibanaConfigAnnotationName
}

func (akb *ApmKibanaAssociation) AssociatedType() string {
	return commonv1.KibanaAssociationType
}

func (akb *ApmKibanaAssociation) AssociationRef() commonv1.ObjectSelector {
	return akb.Spec.KibanaRef.WithDefaultNamespace(akb.Namespace)
}

func (akb *ApmKibanaAssociation) RequiresAssociation() bool {
	return akb.Spec.KibanaRef.Name != ""
}

func (akb *ApmKibanaAssociation) AssociationConf() *commonv1.AssociationConf {
	return akb.kibanaAssocConf
}

func (akb *ApmKibanaAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	akb.kibanaAssocConf = assocConf
}

func (akb *ApmKibanaAssociation) AssociationStatus() commonv1.AssociationStatus {
	return akb.Status.KibanaAssociationStatus
}

func (akb *ApmKibanaAssociation) SetAssociationStatus(status commonv1.AssociationStatus) {
	akb.Status.KibanaAssociationStatus = status
}

var _ commonv1.Associated = &ApmServer{}
