// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// BeatSpec defines the desired state of a Beat.
type BeatSpec struct {
	// Type is the type of the Beat to deploy (filebeat, metricbeat, etc.). Any string can be used,
	// but well-known types will be recognized and will allow to provide sane default configurations.
	// +kubebuilder:validation:MaxLength=20
	Type string `json:"type"`

	// Version of the Beat.
	Version string `json:"version"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// Preset specifies which built-in configuration the operator should use. The configuration provided in a preset
	// consists of: Beat config, roles containing permissions required by that config and podTemplate for DaemonSet
	// or Deployment. Preset must match the Beat `type`.
	// If `deployment` or `daemonSet` is provided it has to match Deployment or DaemonSet in the preset. Then the
	// `podTemplate` is merged with PodTemplate from the preset.
	// If `config` is provided, it replaces the preset config entirely.
	// If preset is not provided, both `config` and `daemonSet` or `deployment` must be specified.
	// +kubebuilder:validation:Optional
	Preset PresetName `json:"preset,omitempty"`

	// Image is the Beat Docker image to deploy. Version and Type have to match the Beat in the image.
	// +kubebuilder:validation:Optional
	Image string `json:"image,omitempty"`

	// Config holds the Beat configuration. If provided, it will override the default configuration.
	// +kubebuilder:validation:Optional
	Config *commonv1.Config `json:"config,omitempty"`

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

// +kubebuilder:validation:Enum=filebeat-k8s-autodiscover;metricbeat-k8s-hosts
type PresetName string

const (
	FilebeatK8sAutodiscoverPreset PresetName = "filebeat-k8s-autodiscover"
	MetricbeatK8sHostsPreset      PresetName = "metricbeat-k8s-hosts"
)

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
	Association commonv1.AssociationStatus `json:"associationStatus,omitempty"`
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

	Spec      BeatSpec                  `json:"spec,omitempty"`
	Status    BeatStatus                `json:"status,omitempty"`
	assocConf *commonv1.AssociationConf `json:"-"` //nolint:govet
}

func (b *Beat) Associated() commonv1.Associated {
	if b != nil {
		return b
	}
	return &Beat{}
}

func (b *Beat) AssociatedType() string {
	return commonv1.ElasticsearchAssociationType
}

func (b *Beat) AssociationRef() commonv1.ObjectSelector {
	return b.Spec.ElasticsearchRef.WithDefaultNamespace(b.Namespace)
}

func (b *Beat) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName
}

func (b *Beat) GetAssociations() []commonv1.Association {
	return []commonv1.Association{b}
}

// IsMarkedForDeletion returns true if the Beat is going to be deleted
func (b *Beat) IsMarkedForDeletion() bool {
	return !b.DeletionTimestamp.IsZero()
}

func (b *Beat) ServiceAccountName() string {
	return b.Spec.ServiceAccountName
}

func (b *Beat) ElasticsearchRef() commonv1.ObjectSelector {
	return b.Spec.ElasticsearchRef
}

func (b *Beat) AssociationConf() *commonv1.AssociationConf {
	return b.assocConf
}

func (b *Beat) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	b.assocConf = assocConf
}

func (b *Beat) AssociationStatus() commonv1.AssociationStatus {
	return b.Status.Association
}

func (b *Beat) SetAssociationStatus(status commonv1.AssociationStatus) {
	b.Status.Association = status
}

var _ commonv1.Associated = &Beat{}
var _ commonv1.Association = &Beat{}

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
