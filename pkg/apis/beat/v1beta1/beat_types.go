// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

var (
	KnownTypes = map[string]struct{}{"filebeat": {}, "metricbeat": {}, "heartbeat": {}, "auditbeat": {}, "journalbeat": {}, "packetbeat": {}}
)

// BeatSpec defines the desired state of a Beat.
type BeatSpec struct {
	// Type is the type of the Beat to deploy (filebeat, metricbeat, heartbeat, auditbeat, journalbeat, packetbeat, etc.).
	// Any string can be used, but well-known types will have the image field defaulted and have the appropriate
	// Elasticsearch roles created automatically. It also allows for dashboard setup when combined with a `KibanaRef`.
	// +kubebuilder:validation:MaxLength=20
	// +kubebuilder:validation:Pattern=[a-zA-Z0-9-]+
	Type string `json:"type"`

	// Version of the Beat.
	Version string `json:"version"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// KibanaRef is a reference to a Kibana instance running in the same Kubernetes cluster.
	// It allows automatic setup of dashboards and visualizations.
	KibanaRef commonv1.ObjectSelector `json:"kibanaRef,omitempty"`

	// Image is the Beat Docker image to deploy. Version and Type have to match the Beat in the image.
	// +kubebuilder:validation:Optional
	Image string `json:"image,omitempty"`

	// Config holds the Beat configuration. At most one of [`Config`, `ConfigRef`] can be specified.
	// +kubebuilder:validation:Optional
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains a reference to an existing Kubernetes Secret holding the Beat configuration.
	// Beat settings must be specified as yaml, under a single "beat.yml" entry. At most one of [`Config`, `ConfigRef`]
	// can be specified.
	// +kubebuilder:validation:Optional
	ConfigRef *commonv1.ConfigSource `json:"configRef,omitempty"`

	// SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Beat.
	// Secrets data can be then referenced in the Beat config using the Secret's keys or as specified in `Entries` field of
	// each SecureSetting.
	// +kubebuilder:validation:Optional
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// DaemonSet specifies the Beat should be deployed as a DaemonSet, and allows providing its spec.
	// Cannot be used along with `deployment`. If both are absent a default for the Type is used.
	// +kubebuilder:validation:Optional
	DaemonSet *DaemonSetSpec `json:"daemonSet,omitempty"`

	// Deployment specifies the Beat should be deployed as a Deployment, and allows providing its spec.
	// Cannot be used along with `daemonSet`. If both are absent a default for the Type is used.
	// +kubebuilder:validation:Optional
	Deployment *DeploymentSpec `json:"deployment,omitempty"`
}

type DaemonSetSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
}

type DeploymentSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	Replicas    *int32                 `json:"replicas,omitempty"`
}

// BeatStatus defines the observed state of a Beat.
type BeatStatus struct {
	// +kubebuilder:validation:Optional
	commonv1.ReconcilerStatus `json:",inline"`

	// +kubebuilder:validation:Optional
	ExpectedNodes int32 `json:"expectedNodes,omitempty"`

	// +kubebuilder:validation:Optional
	Health BeatHealth `json:"health,omitempty"`

	// +kubebuilder:validation:Optional
	ElasticsearchAssociationStatus commonv1.AssociationStatus `json:"elasticsearchAssociationStatus,omitempty"`

	// +kubebuilder:validation:Optional
	KibanaAssocationStatus commonv1.AssociationStatus `json:"kibanaAssociationStatus,omitempty"`
}

type BeatHealth string

const (
	// BeatRedHealth means that the health is neither yellow nor green.
	BeatRedHealth BeatHealth = "red"

	// BeatYellowHealth means that:
	// 1) at least one Pod is Ready, and
	// 2) association is not configured, or configured and established
	BeatYellowHealth BeatHealth = "yellow"

	// BeatGreenHealth means that:
	// 1) all Pods are Ready, and
	// 2) association is not configured, or configured and established
	BeatGreenHealth BeatHealth = "green"
)

// +kubebuilder:object:root=true

// Beat is the Schema for the Beats API.
// +kubebuilder:resource:categories=elastic,shortName=beat
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="expected",type="integer",JSONPath=".status.expectedNodes",description="Expected nodes"
// +kubebuilder:printcolumn:name="type",type="string",JSONPath=".spec.type",description="Beat type"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Beat version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Beat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec        BeatSpec                  `json:"spec,omitempty"`
	Status      BeatStatus                `json:"status,omitempty"`
	esAssocConf *commonv1.AssociationConf `json:"-"` // nolint:govet
	kbAssocConf *commonv1.AssociationConf `json:"-"` // nolint:govet
}

var _ commonv1.Associated = &Beat{}

func (b *Beat) GetAssociations() []commonv1.Association {
	return []commonv1.Association{
		&BeatESAssociation{Beat: b},
		&BeatKibanaAssociation{Beat: b},
	}
}

func (b *Beat) ServiceAccountName() string {
	return b.Spec.ServiceAccountName
}

// IsMarkedForDeletion returns true if the Beat is going to be deleted
func (b *Beat) IsMarkedForDeletion() bool {
	return !b.DeletionTimestamp.IsZero()
}

func (b *Beat) ElasticsearchRef() commonv1.ObjectSelector {
	return b.Spec.ElasticsearchRef
}

type BeatESAssociation struct {
	*Beat
}

var _ commonv1.Association = &BeatESAssociation{}

func (b *BeatESAssociation) Associated() commonv1.Associated {
	if b == nil {
		return nil
	}
	if b.Beat == nil {
		b.Beat = &Beat{}
	}
	return b.Beat
}

func (b *BeatESAssociation) AssociatedType() string {
	return commonv1.ElasticsearchAssociationType
}

func (b *BeatESAssociation) AssociationRef() commonv1.ObjectSelector {
	return b.Spec.ElasticsearchRef.WithDefaultNamespace(b.Namespace)
}

func (b *BeatESAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName
}

func (b *BeatESAssociation) AssociationConf() *commonv1.AssociationConf {
	return b.esAssocConf
}

func (b *BeatESAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	b.esAssocConf = conf
}

func (b *BeatESAssociation) AssociationStatus() commonv1.AssociationStatus {
	return b.Status.ElasticsearchAssociationStatus
}

func (b *BeatESAssociation) SetAssociationStatus(status commonv1.AssociationStatus) {
	b.Status.ElasticsearchAssociationStatus = status
}

type BeatKibanaAssociation struct {
	*Beat
}

var _ commonv1.Association = &BeatKibanaAssociation{}

func (b *BeatKibanaAssociation) AssociationConf() *commonv1.AssociationConf {
	return b.kbAssocConf
}

func (b *BeatKibanaAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	b.kbAssocConf = conf
}

func (b *BeatKibanaAssociation) AssociationStatus() commonv1.AssociationStatus {
	return b.Status.KibanaAssocationStatus
}

func (b *BeatKibanaAssociation) SetAssociationStatus(status commonv1.AssociationStatus) {
	b.Status.KibanaAssocationStatus = status
}

func (b *BeatKibanaAssociation) Associated() commonv1.Associated {
	if b == nil {
		return nil
	}
	if b.Beat == nil {
		b.Beat = &Beat{}
	}
	return b.Beat
}

func (b *BeatKibanaAssociation) AssociatedType() string {
	return commonv1.KibanaAssociationType
}

func (b *BeatKibanaAssociation) AssociationRef() commonv1.ObjectSelector {
	return b.Spec.KibanaRef.WithDefaultNamespace(b.Namespace)
}

func (b *BeatKibanaAssociation) AssociationConfAnnotationName() string {
	return commonv1.KibanaConfigAnnotationName
}

func (b *Beat) SecureSettings() []commonv1.SecretSource {
	return b.Spec.SecureSettings
}

var _ commonv1.Associated = &Beat{}

// +kubebuilder:object:root=true

// BeatList contains a list of Beats.
type BeatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Beat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Beat{}, &BeatList{})
}
