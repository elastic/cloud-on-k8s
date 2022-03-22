// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Beat"
)

var (
	KnownTypes = map[string]struct{}{"filebeat": {}, "metricbeat": {}, "heartbeat": {}, "auditbeat": {}, "journalbeat": {}, "packetbeat": {}}
)

// BeatSpec defines the desired state of a Beat.
type BeatSpec struct {
	// Type is the type of the Beat to deploy (filebeat, metricbeat, heartbeat, auditbeat, journalbeat, packetbeat, and so on).
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
	// +kubebuilder:pruning:PreserveUnknownFields
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
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	// +kubebuilder:validation:Optional
	UpdateStrategy appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type DeploymentSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	Replicas    *int32                 `json:"replicas,omitempty"`
	// +kubebuilder:validation:Optional
	Strategy appsv1.DeploymentStrategy `json:"strategy,omitempty"`
}

// BeatStatus defines the observed state of a Beat.
type BeatStatus struct {
	// Version of the stack resource currently running. During version upgrades, multiple versions may run
	// in parallel: this value specifies the lowest version currently running.
	Version string `json:"version,omitempty"`

	// +kubebuilder:validation:Optional
	ExpectedNodes int32 `json:"expectedNodes,omitempty"`
	// +kubebuilder:validation:Optional
	AvailableNodes int32 `json:"availableNodes,omitempty"`

	// +kubebuilder:validation:Optional
	Health BeatHealth `json:"health,omitempty"`

	// +kubebuilder:validation:Optional
	ElasticsearchAssociationStatus commonv1.AssociationStatus `json:"elasticsearchAssociationStatus,omitempty"`

	// +kubebuilder:validation:Optional
	KibanaAssociationStatus commonv1.AssociationStatus `json:"kibanaAssociationStatus,omitempty"`
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
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Beat version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Beat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec        BeatSpec                  `json:"spec,omitempty"`
	Status      BeatStatus                `json:"status,omitempty"`
	esAssocConf *commonv1.AssociationConf `json:"-"`
	kbAssocConf *commonv1.AssociationConf `json:"-"`
}

func (b *Beat) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		if b.Spec.ElasticsearchRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(b.Status.ElasticsearchAssociationStatus)
		}
	case commonv1.KibanaAssociationType:
		if b.Spec.KibanaRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(b.Status.KibanaAssociationStatus)
		}
	}

	return commonv1.AssociationStatusMap{}
}

func (b *Beat) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	single, err := status.Single()
	if err != nil {
		return err
	}

	switch typ {
	case commonv1.ElasticsearchAssociationType:
		b.Status.ElasticsearchAssociationStatus = single
		return nil
	case commonv1.KibanaAssociationType:
		b.Status.KibanaAssociationStatus = single
		return nil
	default:
		return fmt.Errorf("association type %s not known", typ)
	}
}

var _ commonv1.Associated = &Beat{}

func (b *Beat) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (b *Beat) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)

	if b.Spec.ElasticsearchRef.IsDefined() {
		associations = append(associations, &BeatESAssociation{
			Beat: b,
		})
	}
	if b.Spec.KibanaRef.IsDefined() {
		associations = append(associations, &BeatKibanaAssociation{
			Beat: b,
		})
	}

	return associations
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

func (b *BeatESAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (b *BeatESAssociation) AssociationRef() commonv1.ObjectSelector {
	return b.Spec.ElasticsearchRef.WithDefaultNamespace(b.Namespace)
}

func (b *BeatESAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationNameBase
}

func (b *BeatESAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(b, b.esAssocConf)
}

func (b *BeatESAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	b.esAssocConf = conf
}

func (b *BeatESAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
}

type BeatKibanaAssociation struct {
	*Beat
}

var _ commonv1.Association = &BeatKibanaAssociation{}

func (b *BeatKibanaAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(b, b.kbAssocConf)
}

func (b *BeatKibanaAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	b.kbAssocConf = conf
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

func (b *BeatKibanaAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.KibanaAssociationType
}

func (b *BeatKibanaAssociation) AssociationRef() commonv1.ObjectSelector {
	return b.Spec.KibanaRef.WithDefaultNamespace(b.Namespace)
}

func (b *BeatKibanaAssociation) AssociationConfAnnotationName() string {
	return commonv1.FormatNameWithID(commonv1.KibanaConfigAnnotationNameBase+"%s", b.AssociationID())
}

func (b *BeatKibanaAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
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
