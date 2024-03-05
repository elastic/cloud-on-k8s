// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"fmt"

	"github.com/blang/semver/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

const (
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Agent"
	// FleetServerServiceAccount is the Elasticsearch service account to be used to authenticate.
	FleetServerServiceAccount commonv1.ServiceAccountName = "fleet-server"
)

var (
	MandatoryPolicyIDVersion = version.MustParse("9.0.0-SNAPSHOT")
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
	// +kubebuilder:pruning:PreserveUnknownFields
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

	// ServiceAccountName is used to check access from the current resource to an Elasticsearch resource in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// DaemonSet specifies the Agent should be deployed as a DaemonSet, and allows providing its spec.
	// Cannot be used along with `deployment` or `statefulSet`.
	// +kubebuilder:validation:Optional
	DaemonSet *DaemonSetSpec `json:"daemonSet,omitempty"`

	// Deployment specifies the Agent should be deployed as a Deployment, and allows providing its spec.
	// Cannot be used along with `daemonSet` or `statefulSet`.
	// +kubebuilder:validation:Optional
	Deployment *DeploymentSpec `json:"deployment,omitempty"`

	// StatefulSet specifies the Agent should be deployed as a StatefulSet, and allows providing its spec.
	// Cannot be used along with `daemonSet` or `deployment`.
	// +kubebuilder:validation:Optional
	StatefulSet *StatefulSetSpec `json:"statefulSet,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying DaemonSet or Deployment or StatefulSet.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// HTTP holds the HTTP layer configuration for the Agent in Fleet mode with Fleet Server enabled.
	// +kubebuilder:validation:Optional
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// Mode specifies the source of configuration for the Agent. The configuration can be specified locally through
	// `config` or `configRef` (`standalone` mode), or come from Fleet during runtime (`fleet` mode).
	// Defaults to `standalone` mode.
	// +kubebuilder:validation:Optional
	Mode AgentMode `json:"mode,omitempty"`

	// FleetServerEnabled determines whether this Agent will launch Fleet Server. Don't set unless `mode` is set to `fleet`.
	// +kubebuilder:validation:Optional
	FleetServerEnabled bool `json:"fleetServerEnabled,omitempty"`

	// PolicyID determines into which Agent Policy this Agent will be enrolled.
	// This field will become mandatory in a future release, default policies are deprecated since 8.1.0.
	// +kubebuilder:validation:Optional
	PolicyID string `json:"policyID,omitempty"`

	// KibanaRef is a reference to Kibana where Fleet should be set up and this Agent should be enrolled. Don't set
	// unless `mode` is set to `fleet`.
	// +kubebuilder:validation:Optional
	KibanaRef commonv1.ObjectSelector `json:"kibanaRef,omitempty"`

	// FleetServerRef is a reference to Fleet Server that this Agent should connect to to obtain it's configuration.
	// Don't set unless `mode` is set to `fleet`.
	// +kubebuilder:validation:Optional
	FleetServerRef commonv1.ObjectSelector `json:"fleetServerRef,omitempty"`
}

type Output struct {
	commonv1.ObjectSelector `json:",omitempty,inline"`
	OutputName              string `json:"outputName,omitempty"`
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

type StatefulSetSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	Replicas    *int32                 `json:"replicas,omitempty"`
	ServiceName string                 `json:"serviceName,omitempty"`

	// PodManagementPolicy controls how pods are created during initial scale up,
	// when replacing pods on nodes, or when scaling down. The default policy is
	// `Parallel`, where pods are created in parallel to match the desired scale
	// without waiting, and on scale down will delete all pods at once.
	// The alternative policy is `OrderedReady`, the default for vanilla kubernetes
	// StatefulSets, where pods are created in increasing order in increasing order
	// (pod-0, then pod-1, etc.) and the controller will wait until each pod is ready before
	// continuing. When scaling down, the pods are removed in the opposite order.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=OrderedReady;Parallel
	// +kubebuilder:default:=Parallel
	PodManagementPolicy appsv1.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`

	// VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod.
	// Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.
	// Items defined here take precedence over any default claims added by the operator with the same name.
	// +kubebuilder:validation:Optional
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
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

	// +kubebuilder:validation:Optional
	KibanaAssociationStatus commonv1.AssociationStatus `json:"kibanaAssociationStatus,omitempty"`

	// +kubebuilder:validation:Optional
	FleetServerAssociationStatus commonv1.AssociationStatus `json:"fleetServerAssociationStatus,omitempty"`

	// ObservedGeneration is the most recent generation observed for this Elastic Agent.
	// It corresponds to the metadata generation, which is updated on mutation by the API Server.
	// If the generation observed in status diverges from the generation in metadata, the Elastic
	// Agent controller has not yet processed the changes contained in the Elastic Agent specification.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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

// +kubebuilder:validation:Enum=standalone;fleet

type AgentMode string

const (
	// AgentStandaloneMode denotes running the Agent as standalone.
	AgentStandaloneMode AgentMode = "standalone"

	// AgentFleetMode denotes running the Agent using Fleet.
	AgentFleetMode AgentMode = "fleet"
)

// FleetModeEnabled returns true iff the Agent is running in fleet mode.
func (a AgentSpec) FleetModeEnabled() bool {
	return a.Mode == AgentFleetMode
}

// StandaloneModeEnabled returns true iff the Agent is running in standalone mode. Takes into the account the default.
func (a AgentSpec) StandaloneModeEnabled() bool {
	return a.Mode == "" || a.Mode == AgentStandaloneMode
}

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

	Spec         AgentSpec                                            `json:"spec,omitempty"`
	Status       AgentStatus                                          `json:"status,omitempty"`
	esAssocConfs map[commonv1.ObjectSelector]commonv1.AssociationConf `json:"-"`
	kbAssocConf  *commonv1.AssociationConf                            `json:"-"`
	fsAssocConf  *commonv1.AssociationConf                            `json:"-"`
}

// +kubebuilder:object:root=true

var _ commonv1.Associated = &Agent{}

var FleetServerServiceAccountMinVersion = semver.MustParse("7.17.0")

func (a *Agent) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range a.Spec.ElasticsearchRefs {
		associations = append(associations, &AgentESAssociation{
			Agent: a,
			ref:   ref.WithDefaultNamespace(a.Namespace),
		})
	}

	if a.Spec.KibanaRef.IsDefined() {
		associations = append(associations, &AgentKibanaAssociation{
			Agent: a,
		})
	}

	if a.Spec.FleetServerRef.IsDefined() {
		associations = append(associations, &AgentFleetServerAssociation{
			Agent: a,
		})
	}

	return associations
}

func (a *Agent) ElasticsearchRefs() []commonv1.ObjectSelector {
	refs := make([]commonv1.ObjectSelector, len(a.Spec.ElasticsearchRefs))
	for i, r := range a.Spec.ElasticsearchRefs {
		refs[i] = r.ObjectSelector
	}
	return refs
}

func (a *Agent) ServiceAccountName() string {
	return a.Spec.ServiceAccountName
}

// IsMarkedForDeletion returns true if the Agent is going to be deleted
func (a *Agent) IsMarkedForDeletion() bool {
	return !a.DeletionTimestamp.IsZero()
}

func (a *Agent) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		return a.Status.ElasticsearchAssociationsStatus
	case commonv1.KibanaAssociationType:
		if a.Spec.KibanaRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(a.Status.KibanaAssociationStatus)
		}
	case commonv1.FleetServerAssociationType:
		if a.Spec.FleetServerRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(a.Status.FleetServerAssociationStatus)
		}
	}

	return commonv1.AssociationStatusMap{}
}

func (a *Agent) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		a.Status.ElasticsearchAssociationsStatus = status
		return nil
	case commonv1.KibanaAssociationType:
		single, err := status.Single()
		if err != nil {
			return err
		}
		a.Status.KibanaAssociationStatus = single
		return nil
	case commonv1.FleetServerAssociationType:
		single, err := status.Single()
		if err != nil {
			return err
		}
		a.Status.FleetServerAssociationStatus = single
		return nil
	default:
		return fmt.Errorf("association type %s not known", typ)
	}
}

func (a *Agent) SecureSettings() []commonv1.SecretSource {
	return a.Spec.SecureSettings
}

// GetObservedGeneration will return the observedGeneration from the Elastic Agent's status.
func (a *Agent) GetObservedGeneration() int64 {
	return a.Status.ObservedGeneration
}

type AgentESAssociation struct {
	*Agent
	// ref is the object selector of the Elasticsearch used in Association
	ref commonv1.ObjectSelector
}

var _ commonv1.Association = &AgentESAssociation{}

func (aea *AgentESAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	if !aea.Spec.FleetServerEnabled {
		return "", nil
	}
	v, err := version.Parse(aea.Spec.Version)
	if err != nil {
		return "", err
	}
	if v.GTE(FleetServerServiceAccountMinVersion) {
		return FleetServerServiceAccount, nil
	}
	return "", nil
}

func (aea *AgentESAssociation) AssociationID() string {
	return fmt.Sprintf("%s-%s", aea.ref.Namespace, aea.ref.NameOrSecretName())
}

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
	return aea.ref
}

func (aea *AgentESAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName(aea.ref)
}

func (aea *AgentESAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConfByRef(aea, aea.ref, aea.esAssocConfs)
}

func (aea *AgentESAssociation) SupportsAuthAPIKey() bool {
	return true
}

func (aea *AgentESAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	if aea.esAssocConfs == nil {
		aea.esAssocConfs = make(map[commonv1.ObjectSelector]commonv1.AssociationConf)
	}
	if conf != nil {
		aea.esAssocConfs[aea.ref] = *conf
	}
}

type AgentKibanaAssociation struct {
	*Agent
}

var _ commonv1.Association = &AgentKibanaAssociation{}

func (a *AgentKibanaAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (a *AgentKibanaAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(a, a.kbAssocConf)
}

func (a *AgentKibanaAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	a.kbAssocConf = conf
}

func (a *AgentKibanaAssociation) Associated() commonv1.Associated {
	if a == nil {
		return nil
	}
	if a.Agent == nil {
		a.Agent = &Agent{}
	}
	return a.Agent
}

func (a *AgentKibanaAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.KibanaAssociationType
}

func (a *AgentKibanaAssociation) AssociationRef() commonv1.ObjectSelector {
	return a.Spec.KibanaRef.WithDefaultNamespace(a.Namespace)
}

func (a *AgentKibanaAssociation) AssociationConfAnnotationName() string {
	return commonv1.FormatNameWithID(commonv1.KibanaConfigAnnotationNameBase+"%s", a.AssociationID())
}

func (a *AgentKibanaAssociation) SupportsAuthAPIKey() bool {
	return false
}

func (a *AgentKibanaAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
}

type AgentFleetServerAssociation struct {
	*Agent
}

var _ commonv1.Association = &AgentFleetServerAssociation{}

func (a *AgentFleetServerAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (a *AgentFleetServerAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(a, a.fsAssocConf)
}

func (a *AgentFleetServerAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	a.fsAssocConf = conf
}

func (a *AgentFleetServerAssociation) Associated() commonv1.Associated {
	if a == nil {
		return nil
	}
	if a.Agent == nil {
		a.Agent = &Agent{}
	}
	return a.Agent
}

func (a *AgentFleetServerAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.FleetServerAssociationType
}

func (a *AgentFleetServerAssociation) AssociationRef() commonv1.ObjectSelector {
	return a.Spec.FleetServerRef.WithDefaultNamespace(a.Namespace)
}

func (a *AgentFleetServerAssociation) AssociationConfAnnotationName() string {
	return commonv1.FormatNameWithID(commonv1.FleetServerConfigAnnotationNameBase+"%s", a.AssociationID())
}

func (a *AgentFleetServerAssociation) SupportsAuthAPIKey() bool {
	return false
}

func (a *AgentFleetServerAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
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
