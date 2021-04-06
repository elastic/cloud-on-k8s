// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MapsContainerName = "maps"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "MapsServer"
)

// MapsSpec holds the specification of a Elastic Maps Server instance.
type MapsSpec struct {
	// Version of Elastic Maps Server.
	Version string `json:"version"`

	// Image is the Elastic Maps Server Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of Elastic Maps Server instances to deploy.
	Count int32 `json:"count,omitempty"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Config holds the MapsServer configuration. See: https://www.elastic.co/guide/en/kibana/current/maps-connect-to-ems.html#elastic-maps-server-configuration
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains a reference to an existing Kubernetes Secret holding the Elastic Maps Server configuration.
	// Configuration settings are merged and have precedence over settings specified in `config`.
	// +kubebuilder:validation:Optional
	ConfigRef *commonv1.ConfigSource `json:"configRef,omitempty"`

	// HTTP holds the HTTP layer configuration for Elastic Maps Server.
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Elastic Maps Server pods
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (eg. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// MapsStatus defines the observed state of Elastic Maps Server
type MapsStatus struct {
	commonv1.DeploymentStatus `json:",inline"`
	AssociationStatus         commonv1.AssociationStatus `json:"associationStatus,omitempty"`
}

// IsMarkedForDeletion returns true if the Elastic Maps Server instance is going to be deleted
func (m *MapsServer) IsMarkedForDeletion() bool {
	return !m.DeletionTimestamp.IsZero()
}

func (m *MapsServer) Associated() commonv1.Associated {
	return m
}

func (m *MapsServer) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationNameBase
}

func (m *MapsServer) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (m *MapsServer) AssociationRef() commonv1.ObjectSelector {
	return m.Spec.ElasticsearchRef.WithDefaultNamespace(m.Namespace)
}

func (m *MapsServer) ServiceAccountName() string {
	return m.Spec.ServiceAccountName
}

func (m *MapsServer) AssociationConf() *commonv1.AssociationConf {
	return m.assocConf
}

func (m *MapsServer) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	m.assocConf = assocConf
}

// RequiresAssociation returns true if the spec specifies an Elasticsearch reference.
func (m *MapsServer) RequiresAssociation() bool {
	return m.Spec.ElasticsearchRef.Name != ""
}

func (m *MapsServer) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	if typ == commonv1.ElasticsearchAssociationType && m.Spec.ElasticsearchRef.IsDefined() {
		return commonv1.NewSingleAssociationStatusMap(m.Status.AssociationStatus)
	}

	return commonv1.AssociationStatusMap{}
}

func (m *MapsServer) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	single, err := status.Single()
	if err != nil {
		return err
	}

	if typ != commonv1.ElasticsearchAssociationType {
		return fmt.Errorf("association type %s not known", typ)
	}

	m.Status.AssociationStatus = single
	return nil
}

func (m *MapsServer) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	if m.Spec.ElasticsearchRef.IsDefined() {
		associations = append(associations, m)
	}
	return associations
}

func (m *MapsServer) AssociationID() string {
	return commonv1.SingletonAssociationID
}

var _ commonv1.Associated = &MapsServer{}
var _ commonv1.Association = &MapsServer{}

// +kubebuilder:object:root=true

// MapsServer represents a Elastic Map Server resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=ems
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="MapsServer version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type MapsServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec      MapsSpec                  `json:"spec,omitempty"`
	Status    MapsStatus                `json:"status,omitempty"`
	assocConf *commonv1.AssociationConf `json:"-"`
}

// +kubebuilder:object:root=true

// MapsServerList contains a list of MapsServer
type MapsServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MapsServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MapsServer{}, &MapsServerList{})
}
