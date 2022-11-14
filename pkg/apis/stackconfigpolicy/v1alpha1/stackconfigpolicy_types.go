// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "StackConfigPolicy"

	unknownVersion = 0
)

func init() {
	SchemeBuilder.Register(&StackConfigPolicy{}, &StackConfigPolicyList{})
}

// +kubebuilder:object:root=true

// StackConfigPolicy represents a StackConfigPolicy resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=scp
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.readyCount",description="Resources configured"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type StackConfigPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StackConfigPolicySpec   `json:"spec,omitempty"`
	Status StackConfigPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StackConfigPolicyList contains a list of StackConfigPolicy resources.
type StackConfigPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StackConfigPolicy `json:"items"`
}

type StackConfigPolicySpec struct {
	ResourceSelector metav1.LabelSelector          `json:"resourceSelector,omitempty"`
	SecureSettings   []commonv1.SecretSource       `json:"secureSettings,omitempty"`
	Elasticsearch    ElasticsearchConfigPolicySpec `json:"elasticsearch,omitempty"`
}

type ElasticsearchConfigPolicySpec struct {
	// ClusterSettings holds the Elasticsearch cluster settings (/_cluster/settings)
	// +kubebuilder:pruning:PreserveUnknownFields
	ClusterSettings *commonv1.Config `json:"clusterSettings,omitempty"`
	// SnapshotRepositories holds the Snapshot Repositories settings (/_snapshot)
	// +kubebuilder:pruning:PreserveUnknownFields
	SnapshotRepositories *commonv1.Config `json:"snapshotRepositories,omitempty"`
	// SnapshotLifecyclePolicies holds the Snapshot Lifecycle Policies settings (/_slm/policy)
	// +kubebuilder:pruning:PreserveUnknownFields
	SnapshotLifecyclePolicies *commonv1.Config `json:"snapshotLifecyclePolicies,omitempty"`
	// SecurityRoleMappings holds the Role Mappings settings (/_security/role_mapping)
	// +kubebuilder:pruning:PreserveUnknownFields
	// SecurityRoleMappings *commonv1.Config `json:"securityRoleMapping,omitempty"`
	// AutoscalingPolicies holds the Autoscaling Policies settings (/_autoscaling/policy)
	// +kubebuilder:pruning:PreserveUnknownFields
	// AutoscalingPolicies *commonv1.Config `json:"autoscalingPolicies,omitempty"`
	// IndexLifecyclePolicies holds the Index Lifecycle policies settings (/_ilm/policy)
	// +kubebuilder:pruning:PreserveUnknownFields
	// IndexLifecyclePolicies *commonv1.Config `json:"indexLifecyclePolicies,omitempty"`
	// IngestPipelines holds the Ingest Pipelines settings (/_ingest/pipeline)
	// +kubebuilder:pruning:PreserveUnknownFields
	// IngestPipelines *commonv1.Config `json:"ingestPipelines,omitempty"`
	// IndexTemplates holds the Index and Component Templates settings
	// IndexTemplates *IndexTemplates `json:"indexTemplates,omitempty"`
}

/*type IndexTemplates struct {
	// ComponentTemplates holds the Component Templates settings (/_component_template)
	// +kubebuilder:pruning:PreserveUnknownFields
	ComponentTemplates *commonv1.Config `json:"componentTemplates,omitempty"`
	// ComposableIndexTemplates holds the Index Templates settings (/_index_template)
	// +kubebuilder:pruning:PreserveUnknownFields
	ComposableIndexTemplates *commonv1.Config `json:"composableIndexTemplates,omitempty"`
}*/

type StackConfigPolicyStatus struct {
	// ResourcesStatuses holds the status for each resource to be configured.
	ResourcesStatuses map[string]ResourcePolicyStatus `json:"resourcesStatuses"`
	// Resources is the number of resources to be configured.
	Resources int `json:"resources,omitempty"`
	// Ready is the number of resources successfully configured.
	Ready int `json:"ready,omitempty"`
	// Errors is the number of resources which have an incorrect configuration
	Error int `json:"errors,omitempty"`
	// ReadyCount is a Human representation of the number of resources successfully configured.
	ReadyCount string `json:"readyCount,omitempty"`
	// Phase is the phase of the StackConfigPolicy.
	Phase PolicyPhase `json:"phase,omitempty"`
	// ObservedGeneration is the most recent generation observed for this StackConfigPolicy.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type PolicyPhase string

const (
	UnknownPhase         PolicyPhase = "Unknown"
	ReadyPhase           PolicyPhase = "Ready"
	ApplyingChangesPhase PolicyPhase = "ApplyingChanges"
	InvalidPhase         PolicyPhase = "Invalid"
	ErrorPhase           PolicyPhase = "Error"
	ConflictPhase        PolicyPhase = "Conflict"
)

// phaseOrder
var phaseOrder = map[PolicyPhase]int{
	UnknownPhase:         -1,
	ReadyPhase:           0,
	ApplyingChangesPhase: 1,
	InvalidPhase:         2,
	ErrorPhase:           3,
	ConflictPhase:        4,
}

// ResourcePolicyStatus models the status of the policy for one resource to be configured.
type ResourcePolicyStatus struct {
	Phase           PolicyPhase       `json:"phase,omitempty"`
	CurrentVersion  int64             `json:"currentVersion,omitempty"`
	ExpectedVersion int64             `json:"expectedVersion,omitempty"`
	Error           PolicyStatusError `json:"error,omitempty"`
}

type PolicyStatusError struct {
	Version int64    `json:"version,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

func NewStatus(scp StackConfigPolicy) StackConfigPolicyStatus {
	status := StackConfigPolicyStatus{
		ResourcesStatuses:  map[string]ResourcePolicyStatus{},
		Phase:              UnknownPhase,
		ObservedGeneration: scp.Generation,
	}
	status.setReadyCount()
	return status
}

func (s *StackConfigPolicyStatus) setReadyCount() {
	s.ReadyCount = fmt.Sprintf("%d/%d", s.Ready, s.Resources)
}

func (s *StackConfigPolicyStatus) AddPolicyErrorFor(resource types.NamespacedName, phase PolicyPhase, msg string) {
	s.ResourcesStatuses[resource.String()] = ResourcePolicyStatus{
		Phase: phase,
		Error: PolicyStatusError{Errors: []string{msg}},
	}
	s.Update()
}

func (s *StackConfigPolicyStatus) UpdateResourceStatusPhase(resource types.NamespacedName, status ResourcePolicyStatus) {
	if status.CurrentVersion == unknownVersion { //nolint:gocritic
		status.Phase = UnknownPhase
	} else if status.Error.Errors != nil {
		status.Phase = ErrorPhase
	} else if status.CurrentVersion == status.ExpectedVersion {
		status.Phase = ReadyPhase
	} else {
		status.Phase = ApplyingChangesPhase
	}
	s.ResourcesStatuses[resource.String()] = status
	s.Update()
}

// Update updates the policy status from its resources statuses.
func (s *StackConfigPolicyStatus) Update() {
	s.Resources = len(s.ResourcesStatuses)
	s.Phase = ReadyPhase
	s.Ready = 0
	s.Error = 0
	for _, status := range s.ResourcesStatuses {
		resourcePhase := status.Phase
		if resourcePhase == ReadyPhase {
			s.Ready++
		} else if resourcePhase == ErrorPhase {
			s.Error++
		}
		// update phase if that of the resource status is worse
		if phaseOrder[resourcePhase] > phaseOrder[s.Phase] {
			s.Phase = resourcePhase
		}
	}
	s.setReadyCount()
}

// IsDegraded returns true when the StackConfigPolicyStatus resource is degraded compared to the previous status.
func (s StackConfigPolicyStatus) IsDegraded(prev StackConfigPolicyStatus) bool {
	return prev.Phase == ReadyPhase && s.Phase != ReadyPhase && s.Phase != ApplyingChangesPhase
}

// IsMarkedForDeletion returns true if the StackConfigPolicy resource is going to be deleted.
func (p *StackConfigPolicy) IsMarkedForDeletion() bool {
	return !p.DeletionTimestamp.IsZero()
}
