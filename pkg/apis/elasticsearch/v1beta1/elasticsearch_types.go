// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ElasticsearchContainerName = "elasticsearch"

// ElasticsearchSpec defines the desired state of Elasticsearch
type ElasticsearchSpec struct {
	// Version represents the version of the stack
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// HTTP contains settings for HTTP.
	HTTP commonv1beta1.HTTPConfig `json:"http,omitempty"`

	// NodeSets represents a list of groups of nodes with the same configuration to be part of the cluster
	NodeSets []NodeSet `json:"nodeSets,omitempty"`

	// UpdateStrategy specifies how updates to the cluster should be performed.
	UpdateStrategy UpdateStrategy `json:"updateStrategy,omitempty"`

	// PodDisruptionBudget allows full control of the default pod disruption budget.
	//
	// The default budget selects all cluster pods and sets maxUnavailable to 1.
	// To disable it entirely, set to the empty value (`{}` in YAML).
	// +kubebuilder:validation:Optional
	PodDisruptionBudget *commonv1beta1.PodDisruptionBudgetTemplate `json:"podDisruptionBudget,omitempty"`

	// SecureSettings references secrets containing secure settings, to be injected
	// into Elasticsearch keystore on each node.
	// Each individual key/value entry in the referenced secrets is considered as an
	// individual secure setting to be injected.
	// You can use the `entries` and `key` fields to consider only a subset of the secret
	// entries and the `path` field to change the target path of a secret entry key.
	// The secret must exist in the same namespace as the Elasticsearch resource.
	SecureSettings []commonv1beta1.SecretSource `json:"secureSettings,omitempty"`
}

// NodeCount returns the total number of nodes of the Elasticsearch cluster
func (es ElasticsearchSpec) NodeCount() int32 {
	count := int32(0)
	for _, topoElem := range es.NodeSets {
		count += topoElem.Count
	}
	return count
}

// NodeSet defines a common topology for a set of Elasticsearch nodes
type NodeSet struct {
	// Name is a logical name for this set of nodes. Used as a part of the managed Elasticsearch node.name setting.
	// +kubebuilder:validation:Pattern=[a-zA-Z0-9-]+
	// +kubebuilder:validation:MaxLength=23
	Name string `json:"name"`

	// Config represents Elasticsearch configuration.
	Config *commonv1beta1.Config `json:"config,omitempty"`

	// Count defines how many nodes this topology should have.
	Count int32 `json:"count,omitempty"`

	// PodTemplate can be used to propagate configuration to Elasticsearch pods.
	// This allows specifying custom annotations, labels, environment variables,
	// volumes, affinity, resources, etc. for the pods created from this NodeSet.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// VolumeClaimTemplates is a list of claims that pods are allowed to reference.
	// Every claim in this list must have at least one matching (by name) volumeMount in one
	// container in the template. A claim in this list takes precedence over
	// any volumes in the template, with the same name.
	// TODO: Define the behavior if a claim already exists with the same name.
	// TODO: define special behavior based on claim metadata.name. (e.g data / logs volumes)
	// +kubebuilder:validation:Optional
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// GetESContainerTemplate returns the Elasticsearch container (if set) from the NodeSet's PodTemplate
func (n NodeSet) GetESContainerTemplate() *corev1.Container {
	for _, c := range n.PodTemplate.Spec.Containers {
		if c.Name == ElasticsearchContainerName {
			return &c
		}
	}
	return nil
}

// UpdateStrategy specifies how updates to the cluster should be performed.
type UpdateStrategy struct {
	// ChangeBudget is the change budget that should be used when performing mutations to the cluster.
	ChangeBudget ChangeBudget `json:"changeBudget,omitempty"`
}

// ChangeBudget defines how Pods in a single group should be updated.
type ChangeBudget struct {
	// MaxUnavailable is the maximum number of pods that can be unavailable (not ready) during the update due to the
	// actions controlled by the operator. Setting negative value will result in no restrictions on number of unavailable
	// pods. By default, a fixed value of 1 is used.
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`

	// MaxSurge is the maximum number of pods that can be scheduled above the original number of pods. MaxSurge
	// is only taken into the account when scaling up. Setting negative value will result in no restrictions on number
	// of pods scheduled. By default, it's unbounded.
	MaxSurge *int32 `json:"maxSurge,omitempty"`
}

// DefaultChangeBudget is used when no change budget is provided. It might not be the most effective, but should work in
// most cases.
var DefaultChangeBudget = ChangeBudget{
	MaxSurge:       nil,
	MaxUnavailable: common.Int32(1),
}

func (cb ChangeBudget) GetMaxSurgeOrDefault() *int32 {
	// use default if not specified
	maxSurge := DefaultChangeBudget.MaxSurge
	if cb.MaxSurge != nil {
		maxSurge = cb.MaxSurge
	}

	// nil or negative in the spec denotes unlimited surge
	// in the code unlimited surge is denoted by nil
	if maxSurge == nil || *maxSurge < 0 {
		maxSurge = nil
	}

	return maxSurge
}

func (cb ChangeBudget) GetMaxUnavailableOrDefault() *int32 {
	// use default if not specified
	maxUnavailable := DefaultChangeBudget.MaxUnavailable
	if cb.MaxUnavailable != nil {
		maxUnavailable = cb.MaxUnavailable
	}

	// nil or negative in the spec denotes unlimited unavailability
	// in the code unlimited unavailability is denoted by nil
	if maxUnavailable == nil || *maxUnavailable < 0 {
		maxUnavailable = nil
	}

	return maxUnavailable
}

// ElasticsearchHealth is the health of the cluster as returned by the health API.
type ElasticsearchHealth string

// Possible traffic light states Elasticsearch health can have.
const (
	ElasticsearchRedHealth     ElasticsearchHealth = "red"
	ElasticsearchYellowHealth  ElasticsearchHealth = "yellow"
	ElasticsearchGreenHealth   ElasticsearchHealth = "green"
	ElasticsearchUnknownHealth ElasticsearchHealth = "unknown"
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
	// ElasticsearchReadyPhase is operating at the desired spec.
	ElasticsearchReadyPhase ElasticsearchOrchestrationPhase = "Ready"
	// ElasticsearchApplyingChangesPhase controller is working towards a desired state, cluster can be unavailable.
	ElasticsearchApplyingChangesPhase ElasticsearchOrchestrationPhase = "ApplyingChanges"
	// ElasticsearchMigratingDataPhase Elasticsearch is currently migrating data to another node.
	ElasticsearchMigratingDataPhase ElasticsearchOrchestrationPhase = "MigratingData"
	// ElasticsearchResourceInvalid is marking a resource as invalid, should never happen if admission control is installed correctly.
	ElasticsearchResourceInvalid ElasticsearchOrchestrationPhase = "Invalid"
)

// ElasticsearchStatus defines the observed state of Elasticsearch
type ElasticsearchStatus struct {
	commonv1beta1.ReconcilerStatus `json:",inline"`
	Health                         ElasticsearchHealth             `json:"health,omitempty"`
	Phase                          ElasticsearchOrchestrationPhase `json:"phase,omitempty"`
}

type ZenDiscoveryStatus struct {
	MinimumMasterNodes int `json:"minimumMasterNodes,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (es ElasticsearchStatus) IsDegraded(prev ElasticsearchStatus) bool {
	return es.Health.Less(prev.Health)
}

// +kubebuilder:object:root=true

// Elasticsearch is the Schema for the elasticsearches API
// +kubebuilder:resource:categories=elastic,shortName=es
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version",description="Elasticsearch version"
// +kubebuilder:printcolumn:name="phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Elasticsearch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticsearchSpec   `json:"spec,omitempty"`
	Status ElasticsearchStatus `json:"status,omitempty"`
}

// IsMarkedForDeletion returns true if the Elasticsearch is going to be deleted
func (e Elasticsearch) IsMarkedForDeletion() bool {
	return !e.DeletionTimestamp.IsZero()
}

func (e Elasticsearch) SecureSettings() []commonv1beta1.SecretSource {
	return e.Spec.SecureSettings
}

// +kubebuilder:object:root=true

// ElasticsearchList contains a list of Elasticsearch clusters
type ElasticsearchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Elasticsearch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Elasticsearch{}, &ElasticsearchList{})
}
