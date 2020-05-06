// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const EnterpriseSearchContainerName = "enterprise-search"

// EnterpriseSearchSpec holds the specification of an Enterprise Search resource.
type EnterpriseSearchSpec struct {
	// Version of Enterprise Search.
	Version string `json:"version,omitempty"`

	// Image is the Enterprise Search Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of Enterprise Search instances to deploy.
	Count int32 `json:"count,omitempty"`

	// Config holds the Enterprise Search configuration.
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains references to Kubernetes Secrets holding the Enterprise Search configuration.
	// Configuration settings are merged and have prcedence over plain text settings specified in  `config`.
	// Multiple secrets can be referenced: if duplicate settings exist in multiple secrets,
	// the last one takes precedence.
	ConfigRef []ConfigSource `json:"configRef,omitempty"`

	// HTTP holds the HTTP layer configuration for Enterprise Search resource.
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// ElasticsearchRef is a reference to the Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on)
	// for the Enterprise Search pods.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (eg. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// ConfigSource references configuration settings to include in the Enterprise Search configuration.
type ConfigSource struct {
	// SecretName references a Kubernetes secret in the same namespace as the EnterpriseSearch resource.
	// Enterprise Search settings must be specified as yaml, under a single "enterprise-search.yml" entry.
	//
	// Example:
	// ---
	// kind: Secret
	// apiVersion: v1
	// metadata:
	// 	name: smtp-credentials
	// stringData:
	//  enterprise-search.yml: |-
	//    email.account.enabled: true
	//    email.account.smtp.auth: plain
	//    email.account.smtp.starttls.enable: false
	//    email.account.smtp.host: 127.0.0.1
	//    email.account.smtp.port: 25
	//    email.account.smtp.user: myuser
	//    email.account.smtp.password: mypassword
	//    email.account.email_defaults.from: my@email.com
	// ---
	commonv1.SecretRef `json:",inline"`
}

// EnterpriseSearchHealth expresses the health of the Enterprise Search instances.
type EnterpriseSearchHealth string

const (
	// EnterpriseSearchRed means no instance is currently available.
	EnterpriseSearchRed EnterpriseSearchHealth = "red"
	// EnterpriseSearchGreen means at least one instance is available.
	EnterpriseSearchGreen EnterpriseSearchHealth = "green"
)

// EnterpriseSearchStatus defines the observed state of EnterpriseSearch
type EnterpriseSearchStatus struct {
	commonv1.ReconcilerStatus `json:",inline"`
	Health                    EnterpriseSearchHealth `json:"health,omitempty"`
	// ExternalService is the name of the service associated to the Enterprise Search Pods.
	ExternalService string `json:"service,omitempty"`
	// Association is the status of any auto-linking to Elasticsearch clusters.
	Association commonv1.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (ent EnterpriseSearchStatus) IsDegraded(prev EnterpriseSearchStatus) bool {
	return prev.Health == EnterpriseSearchGreen && ent.Health != EnterpriseSearchGreen
}

// IsMarkedForDeletion returns true if the EnterpriseSearch is going to be deleted
func (ent *EnterpriseSearch) IsMarkedForDeletion() bool {
	return !ent.DeletionTimestamp.IsZero()
}

func (ent *EnterpriseSearch) ServiceAccountName() string {
	return ent.Spec.ServiceAccountName
}

func (ent *EnterpriseSearch) Associated() commonv1.Associated {
	if ent != nil {
		return ent
	}
	return &EnterpriseSearch{}
}

func (ent *EnterpriseSearch) AnnotationName() string {
	return "association.k8s.elastic.co/es-conf"
}

func (ent *EnterpriseSearch) AssociatedServiceType() string {
	return "elasticsearch"
}

func (ent *EnterpriseSearch) AssociationRef() commonv1.ObjectSelector {
	return ent.Spec.ElasticsearchRef
}

func (ent *EnterpriseSearch) AssociationConf() *commonv1.AssociationConf {
	return ent.assocConf
}

func (ent *EnterpriseSearch) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	ent.assocConf = assocConf
}

func (ent *EnterpriseSearch) AssociationStatus() commonv1.AssociationStatus {
	return ent.Status.Association
}

func (ent *EnterpriseSearch) SetAssociationStatus(status commonv1.AssociationStatus) {
	ent.Status.Association = status
}

func (ent *EnterpriseSearch) RequiresAssociation() bool {
	return ent.Spec.ElasticsearchRef.Name != ""
}

func (ent *EnterpriseSearch) GetAssociations() []commonv1.Association {
	return []commonv1.Association{ent}
}

var _ commonv1.Associated = &EnterpriseSearch{}
var _ commonv1.Association = &EnterpriseSearch{}

// +kubebuilder:object:root=true

// EnterpriseSearch is a Kubernetes CRD to represent Enterprise Search.
// +kubebuilder:resource:categories=elastic,shortName=ent
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Enterprise Search version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type EnterpriseSearch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      EnterpriseSearchSpec      `json:"spec,omitempty"`
	Status    EnterpriseSearchStatus    `json:"status,omitempty"`
	assocConf *commonv1.AssociationConf `json:"-"` //nolint:govet
}

// +kubebuilder:object:root=true

// EnterpriseSearchList contains a list of EnterpriseSearch
type EnterpriseSearchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnterpriseSearch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnterpriseSearch{}, &EnterpriseSearchList{})
}
