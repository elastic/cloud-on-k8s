// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const ElasticsearchContainerName = "elasticsearch"

// ElasticsearchSpec holds the specification of an Elasticsearch cluster.
type ElasticsearchSpec struct {
	// Version of Elasticsearch.
	Version string `json:"version"`

	// Image is the Elasticsearch Docker image to deploy.
	Image string `json:"image,omitempty"`

	// HTTP holds HTTP layer settings for Elasticsearch.
	// +kubebuilder:validation:Optional
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// Transport holds transport layer settings for Elasticsearch.
	// +kubebuilder:validation:Optional
	Transport TransportConfig `json:"transport,omitempty"`

	// NodeSets allow specifying groups of Elasticsearch nodes sharing the same configuration and Pod templates.
	// See: https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-orchestration.html
	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`

	// UpdateStrategy specifies how updates to the cluster should be performed.
	// +kubebuilder:validation:Optional
	UpdateStrategy UpdateStrategy `json:"updateStrategy,omitempty"`

	// PodDisruptionBudget provides access to the default pod disruption budget for the Elasticsearch cluster.
	// The default budget selects all cluster pods and sets `maxUnavailable` to 1. To disable, set `PodDisruptionBudget`
	// to the empty value (`{}` in YAML).
	// +kubebuilder:validation:Optional
	PodDisruptionBudget *commonv1.PodDisruptionBudgetTemplate `json:"podDisruptionBudget,omitempty"`

	// Auth contains user authentication and authorization security settings for Elasticsearch.
	// +kubebuilder:validation:Optional
	Auth Auth `json:"auth,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Elasticsearch.
	// See: https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-es-secure-settings.html
	// +kubebuilder:validation:Optional
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (eg. a remote Elasticsearch cluster) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// RemoteClusters enables you to establish uni-directional connections to a remote Elasticsearch cluster.
	// +optional
	RemoteClusters []RemoteCluster `json:"remoteClusters,omitempty"`
}

// TransportConfig holds the transport layer settings for Elasticsearch.
type TransportConfig struct {
	// Service defines the template for the associated Kubernetes Service object.
	Service commonv1.ServiceTemplate `json:"service,omitempty"`
}

// RemoteCluster declares a remote Elasticsearch cluster connection.
type RemoteCluster struct {
	// Name is the name of the remote cluster as it is set in the Elasticsearch settings.
	// The name is expected to be unique for each remote clusters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running within the same k8s cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// TODO: Allow the user to specify some options (transport.compress, transport.ping_schedule)

}

func (r RemoteCluster) ConfigHash() string {
	return hash.HashObject(r)
}

// NodeCount returns the total number of nodes of the Elasticsearch cluster
func (es ElasticsearchSpec) NodeCount() int32 {
	count := int32(0)
	for _, topoElem := range es.NodeSets {
		count += topoElem.Count
	}
	return count
}

// Auth contains user authentication and authorization security settings for Elasticsearch.
type Auth struct {
	// Roles to propagate to the Elasticsearch cluster.
	Roles []RoleSource `json:"roles,omitempty"`
	// FileRealm to propagate to the Elasticsearch cluster.
	FileRealm []FileRealmSource `json:"fileRealm,omitempty"`
}

// RoleSource references roles to create in the Elasticsearch cluster.
type RoleSource struct {
	// SecretName references a Kubernetes secret in the same namespace as the Elasticsearch resource.
	// Multiple roles can be specified in a Kubernetes secret, under a single "roles.yml" entry.
	// The secret value must match the expected file-based specification as described in
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/defining-roles.html#roles-management-file.
	//
	// Example:
	// ---
	// kind: Secret
	// apiVersion: v1
	// metadata:
	// 	name: my-roles
	// stringData:
	//  roles.yml: |-
	//    click_admins:
	//      run_as: [ 'clicks_watcher_1' ]
	//   	cluster: [ 'monitor' ]
	//   	indices:
	//   	- names: [ 'events-*' ]
	//   	  privileges: [ 'read' ]
	//   	  field_security:
	//   		grant: ['category', '@timestamp', 'message' ]
	//   	  query: '{"match": {"category": "click"}}'
	//    another_role:
	//      cluster: [ 'all' ]
	// ---
	commonv1.SecretRef `json:",inline"`
}

// FileRealmSource references users to create in the Elasticsearch cluster.
type FileRealmSource struct {
	// SecretName references a Kubernetes secret in the same namespace as the Elasticsearch resource.
	// Multiple users and their roles mapping can be specified in a Kubernetes secret.
	// The secret should contain 2 entries:
	// - users: contain all users and the hash of their password (https://www.elastic.co/guide/en/elasticsearch/reference/current/security-settings.html#password-hashing-algorithms)
	// - users_roles: contain the role to users mapping
	// The format of those 2 entries must correspond to the expected file realm format, as specified in Elasticsearch
	// documentation: https://www.elastic.co/guide/en/elasticsearch/reference/7.5/file-realm.html#file-realm-configuration.
	//
	// Example:
	// ---
	// # File realm in ES format (from the CLI or manually assembled)
	// kind: Secret
	// apiVersion: v1
	// metadata:
	//   name: my-filerealm
	// stringData:
	//   users: |-
	//     rdeniro:$2a$10$BBJ/ILiyJ1eBTYoRKxkqbuDEdYECplvxnqQ47uiowE7yGqvCEgj9W
	//     alpacino:$2a$10$cNwHnElYiMYZ/T3K4PvzGeJ1KbpXZp2PfoQD.gfaVdImnHOwIuBKS
	//     jacknich:{PBKDF2}50000$z1CLJt0MEFjkIK5iEfgvfnA6xq7lF25uasspsTKSo5Q=$XxCVLbaKDimOdyWgLCLJiyoiWpA/XDMe/xtVgn1r5Sg=
	//   users_roles: |-
	//     admin:rdeniro
	//     power_user:alpacino,jacknich
	//     user:jacknich
	// ---
	commonv1.SecretRef `json:",inline"`
}

// NodeSet is the specification for a group of Elasticsearch nodes sharing the same configuration and a Pod template.
type NodeSet struct {
	// Name of this set of nodes. Becomes a part of the Elasticsearch node.name setting.
	// +kubebuilder:validation:Pattern=[a-zA-Z0-9-]+
	// +kubebuilder:validation:MaxLength=23
	Name string `json:"name"`

	// Config holds the Elasticsearch configuration.
	Config *commonv1.Config `json:"config,omitempty"`

	// Count of Elasticsearch nodes to deploy.
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Pods belonging to this NodeSet.
	// +kubebuilder:validation:Optional
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod in this NodeSet.
	// Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.
	// Items defined here take precedence over any default claims added by the operator with the same name.
	// See: https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-volume-claim-templates.html
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
	// ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.
	ChangeBudget ChangeBudget `json:"changeBudget,omitempty"`
}

// ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.
type ChangeBudget struct {
	// MaxUnavailable is the maximum number of pods that can be unavailable (not ready) during the update due to
	// circumstances under the control of the operator. Setting a negative value will disable this restriction.
	// Defaults to 1 if not specified.
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`

	// MaxSurge is the maximum number of new pods that can be created exceeding the original number of pods defined in
	// the specification. MaxSurge is only taken into consideration when scaling up. Setting a negative value will
	// disable the restriction. Defaults to unbounded if not specified.
	MaxSurge *int32 `json:"maxSurge,omitempty"`
}

// DefaultChangeBudget is used when no change budget is provided. It might not be the most effective, but should work in
// most cases.
var DefaultChangeBudget = ChangeBudget{
	MaxSurge:       nil,
	MaxUnavailable: pointer.Int32(1),
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
	commonv1.ReconcilerStatus `json:",inline"`
	Health                    ElasticsearchHealth             `json:"health,omitempty"`
	Phase                     ElasticsearchOrchestrationPhase `json:"phase,omitempty"`
}

type ZenDiscoveryStatus struct {
	MinimumMasterNodes int `json:"minimumMasterNodes,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (es ElasticsearchStatus) IsDegraded(prev ElasticsearchStatus) bool {
	return es.Health.Less(prev.Health)
}

// +kubebuilder:object:root=true

// Elasticsearch represents an Elasticsearch resource in a Kubernetes cluster.
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
func (es Elasticsearch) IsMarkedForDeletion() bool {
	return !es.DeletionTimestamp.IsZero()
}

func (es Elasticsearch) SecureSettings() []commonv1.SecretSource {
	return es.Spec.SecureSettings
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
