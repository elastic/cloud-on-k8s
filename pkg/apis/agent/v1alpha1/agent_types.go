// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	// Version of the Agent.
	Version string `json:"version"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	ElasticsearchRefs []Output `json:"elasticsearchRefs,omitempty"`

	// Image is the Agent Docker image to deploy. Version has to match the Agent in the image.
	// +kubebuilder:validation:Optional
	Image string `json:"image,omitempty"`

	// Config holds the Agent configuration. At most one of [`Config`, `ConfigRef`] can be specified.
	// +kubebuilder:validation:Optional
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains a reference to an existing Kubernetes Secret holding the Agent configuration.
	// Agent settings must be specified as yaml, under a single "agent.yml" entry. At most one of [`Config`, `ConfigRef`]
	// can be specified.
	// +kubebuilder:validation:Optional
	ConfigRef *commonv1.ConfigSource `json:"configRef,omitempty"`

	// SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Agent.
	// Secrets data can be then referenced in the Agent config using the Secret's keys or as specified in `Entries` field of
	// each SecureSetting.
	// +kubebuilder:validation:Optional
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// DaemonSet specifies the Agent should be deployed as a DaemonSet, and allows providing its spec.
	// Cannot be used along with `deployment`. If both are absent a default for the Type is used.
	// +kubebuilder:validation:Optional
	DaemonSet *DaemonSetSpec `json:"daemonSet,omitempty"`

	// Deployment specifies the Agent should be deployed as a Deployment, and allows providing its spec.
	// Cannot be used along with `daemonSet`. If both are absent a default for the Type is used.
	// +kubebuilder:validation:Optional
	Deployment *DeploymentSpec `json:"deployment,omitempty"`
}

type Output struct {
	commonv1.ObjectSelector `json:",omitempty,inline"`
	OutputName              string `json:"outputName,omitempty"`
}

type DaemonSetSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	Strategy appsv1.DeploymentStrategy `json:"strategy,omitempty"`
}

type DeploymentSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	Replicas    *int32                 `json:"replicas,omitempty"`
	// +kubebuilder:validation:Optional
	Strategy appsv1.DeploymentStrategy `json:"strategy,omitempty"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// Version of the stack resource currently running. During version upgrades, multiple versions may run
	// in parallel: this value specifies the lowest version currently running.
	Version string `json:"version,omitempty"`

	// +kubebuilder:validation:Optional
	ExpectedNodes int32 `json:"expectedNodes,omitempty"`

	// +kubebuilder:validation:Optional
	AvailableNodes int32 `json:"availableNodes,omitempty"`

	// +kubebuilder:validation:Optional
	Health AgentHealth `json:"health,omitempty"`

	// +kubebuilder:validation:Optional
	ElasticsearchAssociationStatus commonv1.AssociationStatus `json:"elasticsearchAssociationStatus,omitempty"`
}

type AgentHealth string

const (
	// AgentRedHealth means that the health is neither yellow nor green.
	AgentRedHealth AgentHealth = "red"

	// AgentYellowHealth means that:
	// 1) at least one Pod is Ready, and
	// 2) association is not configured, or configured and established
	AgentYellowHealth AgentHealth = "yellow"

	// AgentGreenHealth means that:
	// 1) all Pods are Ready, and
	// 2) association is not configured, or configured and established
	AgentGreenHealth AgentHealth = "green"
)

// +kubebuilder:object:root=true

// Agent is the Schema for the Agents API.
// +kubebuilder:resource:categories=elastic,shortName=agent
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:storageversion
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec        AgentSpec                 `json:"spec,omitempty"`
	Status      AgentStatus               `json:"status,omitempty"`
	esAssocConf *commonv1.AssociationConf `json:"-"` // nolint:govet
}

// +kubebuilder:object:root=true

var _ commonv1.Associated = &Agent{}

func (a *Agent) GetAssociations() []commonv1.Association {
	return []commonv1.Association{
		&AgentESAssociation{Agent: a},
	}
}

func (a *Agent) ServiceAccountName() string {
	return a.Spec.ServiceAccountName
}

// IsMarkedForDeletion returns true if the Agent is going to be deleted
func (a *Agent) IsMarkedForDeletion() bool {
	return !a.DeletionTimestamp.IsZero()
}

type AgentESAssociation struct {
	*Agent
}

var _ commonv1.Association = &AgentESAssociation{}

func (a *AgentESAssociation) Associated() commonv1.Associated {
	if a == nil {
		return nil
	}
	if a.Agent == nil {
		a.Agent = &Agent{}
	}
	return a.Agent
}

func (a *AgentESAssociation) AssociatedType() string {
	return commonv1.ElasticsearchAssociationType
}

func (a *AgentESAssociation) AssociationRef() commonv1.ObjectSelector {
	selector := commonv1.ObjectSelector{}
	if len(a.Spec.ElasticsearchRefs) > 0 {
		selector = a.Spec.ElasticsearchRefs[0].ObjectSelector
	}

	return selector.WithDefaultNamespace(a.Namespace)
}

func (a *AgentESAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName
}

func (a *AgentESAssociation) AssociationConf() *commonv1.AssociationConf {
	return a.esAssocConf
}

func (a *AgentESAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	a.esAssocConf = conf
}

func (a *AgentESAssociation) AssociationStatus() commonv1.AssociationStatus {
	return a.Status.ElasticsearchAssociationStatus
}

func (a *AgentESAssociation) SetAssociationStatus(status commonv1.AssociationStatus) {
	a.Status.ElasticsearchAssociationStatus = status
}

func (a *Agent) SecureSettings() []commonv1.SecretSource {
	return a.Spec.SecureSettings
}

var _ commonv1.Associated = &Agent{}

// +kubebuilder:object:root=true

// AgentList contains a list of Agents
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
