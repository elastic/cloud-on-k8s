// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KibanaContainerName = "kibana"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Kibana"
)

// KibanaSpec holds the specification of a Kibana instance.
type KibanaSpec struct {
	// Version of Kibana.
	Version string `json:"version"`

	// Image is the Kibana Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of Kibana instances to deploy.
	Count int32 `json:"count,omitempty"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Config holds the Kibana configuration. See: https://www.elastic.co/guide/en/kibana/current/settings.html
	Config *commonv1.Config `json:"config,omitempty"`

	// HTTP holds the HTTP layer configuration for Kibana.
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Kibana pods
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Kibana.
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (eg. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// KibanaStatus defines the observed state of Kibana
type KibanaStatus struct {
	commonv1.DeploymentStatus `json:",inline"`
	AssociationStatus         commonv1.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsMarkedForDeletion returns true if the Kibana is going to be deleted
func (k *Kibana) IsMarkedForDeletion() bool {
	return !k.DeletionTimestamp.IsZero()
}

func (k *Kibana) Associated() commonv1.Associated {
	return k
}

func (k *Kibana) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationNameBase
}

func (k *Kibana) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (k *Kibana) AssociationRef() commonv1.ObjectSelector {
	return k.Spec.ElasticsearchRef.WithDefaultNamespace(k.Namespace)
}

func (k *Kibana) SecureSettings() []commonv1.SecretSource {
	return k.Spec.SecureSettings
}

func (k *Kibana) ServiceAccountName() string {
	return k.Spec.ServiceAccountName
}

func (k *Kibana) AssociationConf() *commonv1.AssociationConf {
	return k.assocConf
}

func (k *Kibana) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	k.assocConf = assocConf
}

// RequiresAssociation returns true if the spec specifies an Elasticsearch reference.
func (k *Kibana) RequiresAssociation() bool {
	return k.Spec.ElasticsearchRef.Name != ""
}

func (k *Kibana) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	if typ == commonv1.ElasticsearchAssociationType && k.Spec.ElasticsearchRef.IsDefined() {
		return commonv1.NewSingleAssociationStatusMap(k.Status.AssociationStatus)
	}

	return commonv1.AssociationStatusMap{}
}

func (k *Kibana) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	single, err := status.Single()
	if err != nil {
		return err
	}

	if typ != commonv1.ElasticsearchAssociationType {
		return fmt.Errorf("association type %s not known", typ)
	}

	k.Status.AssociationStatus = single
	return nil
}

func (k *Kibana) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	if k.Spec.ElasticsearchRef.IsDefined() {
		associations = append(associations, k)
	}
	return associations
}

func (k *Kibana) AssociationID() string {
	return commonv1.SingletonAssociationID
}

var _ commonv1.Associated = &Kibana{}
var _ commonv1.Association = &Kibana{}

// +kubebuilder:object:root=true

// Kibana represents a Kibana resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=kb
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Kibana version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Kibana struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      KibanaSpec                `json:"spec,omitempty"`
	Status    KibanaStatus              `json:"status,omitempty"`
	assocConf *commonv1.AssociationConf `json:"-"` //nolint:govet
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
