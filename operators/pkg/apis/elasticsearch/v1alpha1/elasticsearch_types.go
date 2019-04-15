// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ElasticsearchSpec defines the desired state of Elasticsearch
type ElasticsearchSpec struct {
	// Version represents the version of the stack
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// LicenseType represents the expected license type for this cluster.
	// Will default to "basic" if not set.
	// +kubebuilder:validation:Enum=basic,trial,gold,platinum
	LicenseType string `json:"licenseType,omitempty"`

	// SetVMMaxMapCount indicates whether an init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	// Defaults to true if not specified. To be disabled, it must be explicitly set to false.
	SetVMMaxMapCount *bool `json:"setVmMaxMapCount,omitempty"`

	// Expose determines which service type to use for this workload. The
	// options are: `ClusterIP|LoadBalancer|NodePort`. Defaults to ClusterIP.
	// +kubebuilder:validation:Enum=ClusterIP,LoadBalancer,NodePort
	Expose string `json:"expose,omitempty"`

	// Nodes represents a list of topology elements to be part of the cluster
	Nodes []NodeSpec `json:"nodes,omitempty"`

	// SnapshotRepository defines a snapshot repository to be used for automatic snapshots.
	SnapshotRepository *SnapshotRepository `json:"snapshotRepository,omitempty"`

	// FeatureFlags are instance-specific flags that enable or disable specific experimental features
	FeatureFlags commonv1alpha1.FeatureFlags `json:"featureFlags,omitempty"`

	// UpdateStrategy specifies how updates to the cluster should be performed.
	UpdateStrategy UpdateStrategy `json:"updateStrategy,omitempty"`
}

// SnapshotRepositoryType as in gcs, AWS s3, file etc.
type SnapshotRepositoryType string

// Supported repository types
const (
	SnapshotRepositoryTypeGCS SnapshotRepositoryType = "gcs"
)

// SnapshotRepositorySettings specify a storage location for snapshots.
type SnapshotRepositorySettings struct {
	// BucketName is the name of the provider specific storage bucket to use.
	BucketName string `json:"bucketName,omitempty"`
	// Credentials is a reference to a secret containing credentials for the storage provider.
	Credentials corev1.SecretReference `json:"credentials,omitempty"`
}

// SnapshotRepository specifies that the user wants automatic snapshots to happen and indicates where they should be stored.
type SnapshotRepository struct {
	// Type of repository
	// +kubebuilder:validation:Enum=gcs
	Type SnapshotRepositoryType `json:"type"`
	// Settings are provider specific repository settings
	Settings SnapshotRepositorySettings `json:"settings"`
}

// NodeCount returns the total number of nodes of the Elasticsearch cluster
func (es ElasticsearchSpec) NodeCount() int32 {
	count := int32(0)
	for _, topoElem := range es.Nodes {
		count += topoElem.NodeCount
	}
	return count
}

// GetLicenseType returns the type of license expected for this cluster.
// If not provided, default to "basic".
func (es ElasticsearchSpec) GetLicenseType() LicenseType {
	licenseType, err := LicenseTypeFromString(es.LicenseType)
	if err != nil {
		return LicenseTypeBasic
	}
	return licenseType
}

// NodeSpec defines a common topology for a set of Elasticsearch nodes
type NodeSpec struct {
	// Config represents Elasticsearch configuration.
	Config Config `json:"config,omitempty"`

	// Resources to be allocated for this topology
	Resources commonv1alpha1.ResourcesSpec `json:"resources,omitempty"`

	// NodeCount defines how many nodes have this topology
	NodeCount int32 `json:"nodeCount,omitempty"`

	// PodTemplate is the object that describes the Elasticsearch pods.
	// +optional
	PodTemplate ElasticsearchPodTemplateSpec `json:"template,omitempty"`

	// VolumeClaimTemplates is a list of claims that pods are allowed to reference.
	// Every claim in this list must have at least one matching (by name) volumeMount in one
	// container in the template. A claim in this list takes precedence over
	// any volumes in the template, with the same name.
	// TODO: Define the behavior if a claim already exists with the same name.
	// TODO: define special behavior based on claim metadata.name. (e.g data / logs volumes)
	// +optional
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// ElasticsearchPodTemplateSpec describes the data a pod should have when created from a template
type ElasticsearchPodTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the pod.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec ElasticsearchPodSpec `json:"spec,omitempty"`
}

type ElasticsearchPodSpec struct {
	// Affinity is the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
}

// UpdateStrategy specifies how updates to the cluster should be performed.
type UpdateStrategy struct {
	// Groups is a list of groups that should have their cluster mutations considered in a fair manner with a strict
	// change budget (not allowing any surge or unavailability) before the entire cluster is reconciled with the
	// full change budget.
	Groups []GroupingDefinition `json:"groups,omitempty"`

	// ChangeBudget is the change budget that should be used when performing mutations to the cluster.
	ChangeBudget *ChangeBudget `json:"changeBudget,omitempty"`
}

// ResolveChangeBudget resolves the optional ChangeBudget into the user-provided one or a defaulted one.
func (s UpdateStrategy) ResolveChangeBudget() ChangeBudget {
	if s.ChangeBudget != nil {
		return *s.ChangeBudget
	}

	return DefaultChangeBudget
}

// GroupingDefinition is used to select a group of pods.
type GroupingDefinition struct {
	// Selector is the selector used to match pods.
	Selector metav1.LabelSelector `json:"selector,omitempty"`
}

// ChangeBudget defines how Pods in a single group should be updated.
type ChangeBudget struct {
	// TODO: MaxUnavailable and MaxSurge would be great to have as intstrs, but due to
	// https://github.com/kubernetes-sigs/kubebuilder/issues/442 this is not currently an option.

	// MaxUnavailable is the maximum number of pods that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of total pods at the start of update (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// This can not be 0 if MaxSurge is 0 if you want automatic rolling changes to be applied.
	// By default, a fixed value of 0 is used.
	// Example: when this is set to 30%, the group can be scaled down by 30%
	// immediately when the rolling update starts. Once new pods are ready, the group
	// can be scaled down further, followed by scaling up the group, ensuring
	// that at least 70% of the target number of pods are available at all times
	// during the update.
	MaxUnavailable int `json:"maxUnavailable"`

	// MaxSurge is the maximum number of pods that can be scheduled above the original number of
	// pods.
	// By default, a fixed value of 1 is used.
	// Value can be an absolute number (ex: 5) or a percentage of total pods at
	// the start of the update (ex: 10%). This can not be 0 if MaxUnavailable is 0 if you want automatic rolling
	// updates to be applied.
	// Absolute number is calculated from percentage by rounding up.
	// Example: when this is set to 30%, the new group can be scaled up by 30%
	// immediately when the rolling update starts. Once old pods have been killed,
	// new group can be scaled up further, ensuring that total number of pods running
	// at any time during the update is at most 130% of the target number of pods.
	MaxSurge int `json:"maxSurge"`
}

// DefaultFallbackGroupingDefinition is the grouping definition that is used if no user-defined groups are specified or
// there are pods that are not selected by the user-defined groups.
var DefaultFallbackGroupingDefinition = GroupingDefinition{
	// use a selector that matches everything
	Selector: metav1.LabelSelector{},
}

// DefaultChangeBudget is used when no change budget is provided. It might not be the most effective, but should work in
// every case
var DefaultChangeBudget = ChangeBudget{
	MaxSurge:       1,
	MaxUnavailable: 0,
}

// ElasticsearchHealth is the health of the cluster as returned by the health API.
type ElasticsearchHealth string

// Possible traffic light states Elasticsearch health can have.
const (
	ElasticsearchRedHealth    ElasticsearchHealth = "red"
	ElasticsearchYellowHealth ElasticsearchHealth = "yellow"
	ElasticsearchGreenHealth  ElasticsearchHealth = "green"
)

var elasticsearchHealthOrder = map[ElasticsearchHealth]int{
	ElasticsearchRedHealth:    1,
	ElasticsearchYellowHealth: 2,
	ElasticsearchGreenHealth:  3,
}

// Less for ElasticsearchHealth means green > yellow > red
func (h ElasticsearchHealth) Less(other ElasticsearchHealth) bool {
	l := elasticsearchHealthOrder[h]
	r := elasticsearchHealthOrder[other]
	// 0 is not found/unknown and less is not defined for that
	return l != 0 && r != 0 && l < r
}

// ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.
type ElasticsearchOrchestrationPhase string

const (
	// ElasticsearchOperationalPhase is operating at the desired spec.
	ElasticsearchOperationalPhase ElasticsearchOrchestrationPhase = "Operational"
	// ElasticsearchPendingPhase controller is working towards a desired state, cluster can be unavailable.
	ElasticsearchPendingPhase ElasticsearchOrchestrationPhase = "Pending"
	// ElasticsearchMigratingDataPhase Elasticsearch is currently migrating data to another node.
	ElasticsearchMigratingDataPhase ElasticsearchOrchestrationPhase = "MigratingData"
)

// ElasticsearchStatus defines the observed state of Elasticsearch
type ElasticsearchStatus struct {
	commonv1alpha1.ReconcilerStatus
	Health          ElasticsearchHealth             `json:"health,omitempty"`
	Phase           ElasticsearchOrchestrationPhase `json:"phase,omitempty"`
	ClusterUUID     string                          `json:"clusterUUID,omitempty"`
	MasterNode      string                          `json:"masterNode,omitempty"`
	ExternalService string                          `json:"service,omitempty"`
	RemoteClusters  map[string]string               `json:"remoteClusters,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (es ElasticsearchStatus) IsDegraded(prev ElasticsearchStatus) bool {
	return es.Health.Less(prev.Health)
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Elasticsearch is the Schema for the elasticsearches API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=es
// +kubebuilder:categories=elastic
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Elasticsearch version"
// +kubebuilder:printcolumn:name="phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type Elasticsearch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticsearchSpec   `json:"spec,omitempty"`
	Status ElasticsearchStatus `json:"status,omitempty"`
}

// IsMarkedForDeletion returns true if the Elasticsearch is going to be deleted
func (e Elasticsearch) IsMarkedForDeletion() bool {
	if e.DeletionTimestamp.IsZero() { // already handles nil pointer
		return false
	}
	return true
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ElasticsearchList contains a list of Elasticsearch clusters
type ElasticsearchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Elasticsearch `json:"items"`
}

// TrustRelationshipSpec contains configuration for trust restrictions.
type TrustRelationshipSpec struct {
	// CaCert contains the PEM-encoded CA certificate for the remote cluster.
	CaCert string `json:"caCert,omitempty"`
	// TrustRestrictions contains configuration for the trust restrictions feature of Elasticsearch for this
	// relationship
	TrustRestrictions TrustRestrictions `json:"trustRestrictions,omitempty"`
}

// TrustRestrictions is the trust restrictions of an Elasticsearch cluster.
type TrustRestrictions struct {
	// Trust contains configuration for the Elasticsearch trust restrictions.
	Trust Trust `json:"trust,omitempty"`
}

// Trust contains configuration for the Elasticsearch trust restrictions.
type Trust struct {
	// SubjectName is a list of patterns that incoming TLS client certificates must match.
	SubjectName []string `json:"subjectName,omitempty"`
}

// TrustRelationship describes one direction of the trust relationship between two Elasticsearch clusters.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
type TrustRelationship struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TrustRelationshipSpec `json:"spec,omitempty"`
}

// TrustRelationshipList contains a list of TrustRelationships.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TrustRelationshipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrustRelationship `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&Elasticsearch{}, &ElasticsearchList{},
		&TrustRelationship{}, &TrustRelationshipList{},
	)
}
