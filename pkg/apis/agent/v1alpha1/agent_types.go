// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Agent"
)

// AgentSpec defines the desired state of the Agent
type AgentSpec struct {
	// Version of the Agent.
	Version string `json:"version"`

	// ElasticsearchRefs is a reference to a list of Elasticsearch clusters running in the same Kubernetes cluster.
	// Due to existing limitations, only a single ES cluster is currently supported.
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

	// ServiceAccountName is used to check access from the current resource to a Elasticsearch resource in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// DaemonSet specifies the Agent should be deployed as a DaemonSet, and allows providing its spec.
	// Cannot be used along with `deployment`.
	// +kubebuilder:validation:Optional
	DaemonSet *DaemonSetSpec `json:"daemonSet,omitempty"`

	// Deployment specifies the Agent should be deployed as a Deployment, and allows providing its spec.
	// Cannot be used along with `daemonSet`.
	// +kubebuilder:validation:Optional
	Deployment *DeploymentSpec `json:"deployment,omitempty"`
}

type Output struct {
	commonv1.ObjectSelector `json:",omitempty,inline"`
	OutputName              string `json:"outputName,omitempty"`
}

type DaemonSetSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// +kubebuilder:validation:Optional
	UpdateStrategy appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type DeploymentSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	Replicas    *int32                 `json:"replicas,omitempty"`

	// +kubebuilder:validation:Optional
	Strategy appsv1.DeploymentStrategy `json:"strategy,omitempty"`
}

// AgentStatus defines the observed state of the Agent
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
	ElasticsearchAssociationsStatus commonv1.AssociationStatusMap `json:"elasticsearchAssociationsStatus,omitempty"`
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
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="expected",type="integer",JSONPath=".status.expectedNodes",description="Expected nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Agent version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec       AgentSpec                                         `json:"spec,omitempty"`
	Status     AgentStatus                                       `json:"status,omitempty"`
	assocConfs map[types.NamespacedName]commonv1.AssociationConf `json:"-"` // nolint:govet
}

// +kubebuilder:object:root=true

var _ commonv1.Associated = &Agent{}

func (a *Agent) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range a.Spec.ElasticsearchRefs {
		associations = append(associations, &AgentESAssociation{
			Agent: a,
			ref:   ref.WithDefaultNamespace(a.Namespace).NamespacedName(),
		})
	}

	return associations
}

func (a *Agent) ServiceAccountName() string {
	return a.Spec.ServiceAccountName
}

// IsMarkedForDeletion returns true if the Agent is going to be deleted
func (a *Agent) IsMarkedForDeletion() bool {
	return !a.DeletionTimestamp.IsZero()
}

func (a *Agent) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	if typ != commonv1.ElasticsearchAssociationType {
		return commonv1.AssociationStatusMap{}
	}

	return a.Status.ElasticsearchAssociationsStatus
}

func (a *Agent) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	if typ != commonv1.ElasticsearchAssociationType {
		return fmt.Errorf("association type %s not known", typ)
	}

	a.Status.ElasticsearchAssociationsStatus = status
	return nil
}

func (a *Agent) SecureSettings() []commonv1.SecretSource {
	return a.Spec.SecureSettings
}

type AgentESAssociation struct {
	*Agent
	// ref is the namespaced name of the Elasticsearch used in Association
	ref types.NamespacedName
}

func (aea *AgentESAssociation) AssociationID() string {
	return fmt.Sprintf("%s-%s", aea.ref.Namespace, aea.ref.Name)
}

var _ commonv1.Association = &AgentESAssociation{}

func (aea *AgentESAssociation) Associated() commonv1.Associated {
	if aea == nil {
		return nil
	}
	if aea.Agent == nil {
		aea.Agent = &Agent{}
	}
	return aea.Agent
}

func (aea *AgentESAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (aea *AgentESAssociation) AssociationRef() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      aea.ref.Name,
		Namespace: aea.ref.Namespace,
	}
}

func (aea *AgentESAssociation) AssociationConfAnnotationName() string {
	// annotation key should be stable to allow Agent Controller only pick up the ones it expects,
	// based on ElasticsearchRefs

	nsNameHash := sha256.New224()
	// concat with dot to avoid collisions, as namespace can't contain dots
	_, _ = nsNameHash.Write([]byte(fmt.Sprintf("%s.%s", aea.ref.Namespace, aea.ref.Name)))
	// base32 to encode and limit the length, as using Sprintf with "%x" encodes with base16 which happens to
	// give too long output
	// no padding to avoid illegal '=' character in the annotation name
	hash := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(nsNameHash.Sum(nil))

	return commonv1.FormatNameWithID(
		commonv1.ElasticsearchConfigAnnotationNameBase+"%s",
		hash,
	)
}

func (aea *AgentESAssociation) AssociationConf() *commonv1.AssociationConf {
	if aea.assocConfs == nil {
		return nil
	}
	assocConf, found := aea.assocConfs[aea.ref]
	if !found {
		return nil
	}

	return &assocConf
}

func (aea *AgentESAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	if aea.assocConfs == nil {
		aea.assocConfs = make(map[types.NamespacedName]commonv1.AssociationConf)
	}
	if conf != nil {
		aea.assocConfs[aea.ref] = *conf
	}
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
