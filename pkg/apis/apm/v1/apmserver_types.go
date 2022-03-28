// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	"github.com/blang/semver/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	ApmServerContainerName = "apm-server"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "ApmServer"
)

// ApmServerSpec holds the specification of an APM Server.
type ApmServerSpec struct {
	// Version of the APM Server.
	Version string `json:"version"`

	// Image is the APM Server Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of APM Server instances to deploy.
	Count int32 `json:"count,omitempty"`

	// Config holds the APM Server configuration. See: https://www.elastic.co/guide/en/apm/server/current/configuring-howto-apm-server.html
	// +kubebuilder:pruning:PreserveUnknownFields
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
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for APM Server.
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// ApmServerStatus defines the observed state of ApmServer
type ApmServerStatus struct {
	commonv1.DeploymentStatus `json:",inline"`
	// ExternalService is the name of the service the agents should connect to.
	ExternalService string `json:"service,omitempty"`
	// SecretTokenSecretName is the name of the Secret that contains the secret token
	SecretTokenSecretName string `json:"secretTokenSecret,omitempty"`
	// ElasticsearchAssociationStatus is the status of any auto-linking to Elasticsearch clusters.
	ElasticsearchAssociationStatus commonv1.AssociationStatus `json:"elasticsearchAssociationStatus,omitempty"`
	// KibanaAssociationStatus is the status of any auto-linking to Kibana.
	KibanaAssociationStatus commonv1.AssociationStatus `json:"kibanaAssociationStatus,omitempty"`
}

// +kubebuilder:object:root=true

// ApmServer represents an APM Server resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=apm
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="APM version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:scale:specpath=.spec.count,statuspath=.status.count,selectorpath=.status.selector
// +kubebuilder:storageversion
type ApmServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec            ApmServerSpec             `json:"spec,omitempty"`
	Status          ApmServerStatus           `json:"status,omitempty"`
	esAssocConf     *commonv1.AssociationConf `json:"-"`
	kibanaAssocConf *commonv1.AssociationConf `json:"-"`
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

// EffectiveVersion returns the version as it would be reported by APM server. For development builds
// APM server does not use prerelease or build suffixes.
func (as *ApmServer) EffectiveVersion() string {
	ver, err := semver.FinalizeVersion(as.Spec.Version)
	if err != nil {
		// just pass it back if it's malformed
		return as.Spec.Version
	}

	return ver
}

func (as *ApmServer) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (as *ApmServer) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)

	if as.Spec.ElasticsearchRef.IsDefined() {
		associations = append(associations, &ApmEsAssociation{
			ApmServer: as,
		})
	}
	if as.Spec.KibanaRef.IsDefined() {
		associations = append(associations, &ApmKibanaAssociation{
			ApmServer: as,
		})
	}

	return associations
}

func (as *ApmServer) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		if as.Spec.ElasticsearchRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(as.Status.ElasticsearchAssociationStatus)
		}
	case commonv1.KibanaAssociationType:
		if as.Spec.KibanaRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(as.Status.KibanaAssociationStatus)
		}
	}

	return commonv1.AssociationStatusMap{}
}

func (as *ApmServer) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	single, err := status.Single()
	if err != nil {
		return err
	}

	switch typ {
	case commonv1.ElasticsearchAssociationType:
		as.Status.ElasticsearchAssociationStatus = single
		return nil
	case commonv1.KibanaAssociationType:
		as.Status.KibanaAssociationStatus = single
		return nil
	default:
		return fmt.Errorf("association type %s not known", typ)
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
	return commonv1.ElasticsearchConfigAnnotationNameBase
}

func (aes *ApmEsAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (aes *ApmEsAssociation) AssociationRef() commonv1.ObjectSelector {
	return aes.Spec.ElasticsearchRef.WithDefaultNamespace(aes.Namespace)
}

func (aes *ApmEsAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(aes, aes.esAssocConf)
}

func (aes *ApmEsAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	aes.esAssocConf = assocConf
}

func (aes *ApmEsAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
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
	return commonv1.FormatNameWithID(commonv1.KibanaConfigAnnotationNameBase+"%s", akb.AssociationID())
}

func (akb *ApmKibanaAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.KibanaAssociationType
}

func (akb *ApmKibanaAssociation) AssociationRef() commonv1.ObjectSelector {
	return akb.Spec.KibanaRef.WithDefaultNamespace(akb.Namespace)
}

func (akb *ApmKibanaAssociation) RequiresAssociation() bool {
	return akb.Spec.KibanaRef.Name != ""
}

func (akb *ApmKibanaAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(akb, akb.kibanaAssocConf)
}

func (akb *ApmKibanaAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	akb.kibanaAssocConf = assocConf
}

func (akb *ApmKibanaAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
}

var _ commonv1.Associated = &ApmServer{}
