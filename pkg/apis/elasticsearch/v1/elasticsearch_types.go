// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

const (
	ElasticsearchContainerName = "elasticsearch"
	// DisableUpgradePredicatesAnnotation is the annotation that can be applied to an
	// Elasticsearch cluster to disable certain predicates during rolling upgrades.  Multiple
	// predicates names can be separated by ",".
	//
	// Example:
	//
	//   To disable "if_yellow_only_restart_upgrading_nodes_with_unassigned_replicas" predicate
	//
	//   metadata:
	//     annotations:
	//       eck.k8s.elastic.co/disable-upgrade-predicates="if_yellow_only_restart_upgrading_nodes_with_unassigned_replicas"
	DisableUpgradePredicatesAnnotation = "eck.k8s.elastic.co/disable-upgrade-predicates"
	// DownwardNodeLabelsAnnotation holds an optional list of expected node labels to be set as annotations on the Elasticsearch Pods.
	DownwardNodeLabelsAnnotation = "eck.k8s.elastic.co/downward-node-labels"
	// SuspendAnnotation allows users to annotate the Elasticsearch resource with the names of Pods they want to suspend
	// for debugging purposes.
	SuspendAnnotation = "eck.k8s.elastic.co/suspend"
	// DisableDowngradeValidationAnnotation allows circumventing downgrade/upgrade checks.
	DisableDowngradeValidationAnnotation = "eck.k8s.elastic.co/disable-downgrade-validation"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Elasticsearch"
)

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
	// +kubebuilder:validation:Optional
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (for ex. a remote Elasticsearch cluster) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// RemoteClusters enables you to establish uni-directional connections to a remote Elasticsearch cluster.
	// +optional
	RemoteClusters []RemoteCluster `json:"remoteClusters,omitempty"`

	// VolumeClaimDeletePolicy sets the policy for handling deletion of PersistentVolumeClaims for all NodeSets.
	// Possible values are DeleteOnScaledownOnly and DeleteOnScaledownAndClusterDeletion. Defaults to DeleteOnScaledownAndClusterDeletion.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=DeleteOnScaledownOnly;DeleteOnScaledownAndClusterDeletion
	VolumeClaimDeletePolicy VolumeClaimDeletePolicy `json:"volumeClaimDeletePolicy,omitempty"`

	// Monitoring enables you to collect and ship log and monitoring data of this Elasticsearch cluster.
	// See https://www.elastic.co/guide/en/elasticsearch/reference/current/monitor-elasticsearch-cluster.html.
	// Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different
	// Elasticsearch monitoring clusters running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	Monitoring Monitoring `json:"monitoring,omitempty"`
}

type Monitoring struct {
	// Metrics holds references to Elasticsearch clusters which receive monitoring data from this Elasticsearch cluster.
	// +kubebuilder:validation:Optional
	Metrics MetricsMonitoring `json:"metrics,omitempty"`
	// Logs holds references to Elasticsearch clusters which receive log data from this Elasticsearch cluster.
	// +kubebuilder:validation:Optional
	Logs LogsMonitoring `json:"logs,omitempty"`
}

type MetricsMonitoring struct {
	// ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster.
	// Due to existing limitations, only a single Elasticsearch cluster is currently supported.
	// +kubebuilder:validation:Required
	ElasticsearchRefs []commonv1.ObjectSelector `json:"elasticsearchRefs,omitempty"`
}

type LogsMonitoring struct {
	// ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster.
	// Due to existing limitations, only a single Elasticsearch cluster is currently supported.
	// +kubebuilder:validation:Required
	ElasticsearchRefs []commonv1.ObjectSelector `json:"elasticsearchRefs,omitempty"`
}

// VolumeClaimDeletePolicy describes the delete policy for handling PersistentVolumeClaims that hold Elasticsearch data.
// Inspired by https://github.com/kubernetes/enhancements/pull/2440
type VolumeClaimDeletePolicy string

const (
	// DeleteOnScaledownAndClusterDeletionPolicy remove PersistentVolumeClaims when the corresponding Elasticsearch node is removed.
	DeleteOnScaledownAndClusterDeletionPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownAndClusterDeletion"
	// DeleteOnScaledownOnlyPolicy removes PersistentVolumeClaims on scale down of Elasticsearch nodes but retains all
	// current PersistenVolumeClaims when the Elasticsearch cluster has been deleted.
	DeleteOnScaledownOnlyPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownOnly"
)

// TransportConfig holds the transport layer settings for Elasticsearch.
type TransportConfig struct {
	// Service defines the template for the associated Kubernetes Service object.
	Service commonv1.ServiceTemplate `json:"service,omitempty"`
	// TLS defines options for configuring TLS on the transport layer.
	TLS TransportTLSOptions `json:"tls,omitempty"`
}

type TransportTLSOptions struct {
	// OtherNameSuffix when defined will be prefixed with the Pod name and used as the common name,
	// and the first DNSName, as well as an OtherName required by Elasticsearch in the Subject Alternative Name
	// extension of each Elasticsearch node's transport TLS certificate.
	// Example: if set to "node.cluster.local", the generated certificate will have its otherName set to "<pod_name>.node.cluster.local".
	OtherNameSuffix string `json:"otherNameSuffix,omitempty"`
	// SubjectAlternativeNames is a list of SANs to include in the generated node transport TLS certificates.
	SubjectAlternativeNames []commonv1.SubjectAlternativeName `json:"subjectAltNames,omitempty"`
	// Certificate is a reference to a Kubernetes secret that contains the CA certificate
	// and private key for generating node certificates.
	// The referenced secret should contain the following:
	//
	// - `ca.crt`: The CA certificate in PEM format.
	// - `ca.key`: The private key for the CA certificate in PEM format.
	Certificate commonv1.SecretRef `json:"certificate,omitempty"`
}

func (tto TransportTLSOptions) UserDefinedCA() bool {
	return tto.Certificate.SecretName != ""
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

func (es ElasticsearchSpec) VolumeClaimDeletePolicyOrDefault() VolumeClaimDeletePolicy {
	if es.VolumeClaimDeletePolicy == "" {
		return DeleteOnScaledownAndClusterDeletionPolicy
	}
	return es.VolumeClaimDeletePolicy
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
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`

	// Count of Elasticsearch nodes to deploy.
	// If the node set is managed by an autoscaling policy the initial value is automatically set by the autoscaling controller.
	// +kubebuilder:validation:Optional
	Count int32 `json:"count"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Pods belonging to this NodeSet.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod in this NodeSet.
	// Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.
	// Items defined here take precedence over any default claims added by the operator with the same name.
	// +kubebuilder:validation:Optional
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// +kubebuilder:object:generate=false
type NodeSetList []NodeSet

func (nsl NodeSetList) Names() []string {
	names := make([]string, len(nsl))
	for i := range nsl {
		names[i] = nsl[i].Name
	}
	return names
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

// +kubebuilder:object:root=true

// Elasticsearch represents an Elasticsearch resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=es
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Elasticsearch version"
// +kubebuilder:printcolumn:name="phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
type Elasticsearch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec       ElasticsearchSpec                                 `json:"spec,omitempty"`
	Status     ElasticsearchStatus                               `json:"status,omitempty"`
	AssocConfs map[types.NamespacedName]commonv1.AssociationConf `json:"-"`
}

// DownwardNodeLabels returns the set of expected node labels to be copied as annotations on the Elasticsearch Pods.
func (es Elasticsearch) DownwardNodeLabels() []string {
	expectedAnnotations, exist := es.Annotations[DownwardNodeLabelsAnnotation]
	expectedAnnotations = strings.TrimSpace(expectedAnnotations)
	if !exist || expectedAnnotations == "" {
		return nil
	}
	return strings.Split(expectedAnnotations, ",")
}

// HasDownwardNodeLabels returns true if some node labels are expected on the Elasticsearch Pods.
func (es Elasticsearch) HasDownwardNodeLabels() bool {
	return len(es.DownwardNodeLabels()) > 0
}

// IsMarkedForDeletion returns true if the Elasticsearch is going to be deleted
func (es Elasticsearch) IsMarkedForDeletion() bool {
	return !es.DeletionTimestamp.IsZero()
}

// IsConfiguredToAllowDowngrades returns true if the DisableDowngradeValidation annotation is set to the value of true.
func (es Elasticsearch) IsConfiguredToAllowDowngrades() bool {
	val, exists := es.Annotations[DisableDowngradeValidationAnnotation]
	return exists && val == "true"
}

func (es *Elasticsearch) ServiceAccountName() string {
	return es.Spec.ServiceAccountName
}

func (es *Elasticsearch) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

// IsAutoscalingDefined returns true if there is an autoscaling configuration in the annotations.
func (es Elasticsearch) IsAutoscalingDefined() bool {
	_, ok := es.Annotations[ElasticsearchAutoscalingSpecAnnotationName]
	return ok
}

// AutoscalingSpec returns the autoscaling spec in the Elasticsearch manifest.
func (es Elasticsearch) AutoscalingSpec() string {
	return es.Annotations[ElasticsearchAutoscalingSpecAnnotationName]
}

func (es Elasticsearch) SecureSettings() []commonv1.SecretSource {
	return es.Spec.SecureSettings
}

func (es Elasticsearch) SuspendedPodNames() set.StringSet {
	return setFromAnnotations(SuspendAnnotation, es.Annotations)
}

// GetObservedGeneration will return the observed generation from the Elasticsearch status.
func (es Elasticsearch) GetObservedGeneration() int64 {
	return es.Status.ObservedGeneration
}

func setFromAnnotations(annotationKey string, annotations map[string]string) set.StringSet {
	allValues, exists := annotations[annotationKey]
	if !exists {
		return nil
	}

	splitValues := strings.Split(allValues, ",")
	valueSet := set.Make()
	for _, p := range splitValues {
		valueSet.Add(strings.TrimSpace(p))
	}
	return valueSet
}

// -- associations

var _ commonv1.Associated = &Elasticsearch{}

func (es *Elasticsearch) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range es.Spec.Monitoring.Metrics.ElasticsearchRefs {
		if ref.IsDefined() {
			associations = append(associations, &EsMonitoringAssociation{
				Elasticsearch: es,
				ref:           ref.WithDefaultNamespace(es.Namespace).NamespacedName(),
			})
		}
	}
	for _, ref := range es.Spec.Monitoring.Logs.ElasticsearchRefs {
		if ref.IsDefined() {
			associations = append(associations, &EsMonitoringAssociation{
				Elasticsearch: es,
				ref:           ref.WithDefaultNamespace(es.Namespace).NamespacedName(),
			})
		}
	}
	return associations
}

// -- association with monitoring Elasticsearch clusters

// EsMonitoringAssociation helps to manage Elasticsearch+Metricbeat+Filebeat <-> Elasticsearch(es) associations
type EsMonitoringAssociation struct {
	// The monitored Elasticsearch cluster from where are collected logs and monitoring metrics
	*Elasticsearch
	// ref is the namespaced name of the Elasticsearch referenced in the Association used to send and store monitoring data
	ref types.NamespacedName
}

var _ commonv1.Association = &EsMonitoringAssociation{}

func (ema *EsMonitoringAssociation) Associated() commonv1.Associated {
	if ema == nil {
		return nil
	}
	if ema.Elasticsearch == nil {
		ema.Elasticsearch = &Elasticsearch{}
	}
	return ema.Elasticsearch
}

func (ema *EsMonitoringAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName(ema.ref)
}

func (ema *EsMonitoringAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.EsMonitoringAssociationType
}

func (ema *EsMonitoringAssociation) AssociationRef() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      ema.ref.Name,
		Namespace: ema.ref.Namespace,
	}
}

func (ema *EsMonitoringAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConfByRef(ema, ema.ref, ema.AssocConfs)
}

func (ema *EsMonitoringAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	if ema.AssocConfs == nil {
		ema.AssocConfs = make(map[types.NamespacedName]commonv1.AssociationConf)
	}
	if assocConf != nil {
		ema.AssocConfs[ema.ref] = *assocConf
	}
}

func (ema *EsMonitoringAssociation) AssociationID() string {
	return fmt.Sprintf("%s-%s", ema.ref.Namespace, ema.ref.Name)
}

// HasMonitoring methods

func (es *Elasticsearch) GetMonitoringMetricsRefs() []commonv1.ObjectSelector {
	return es.Spec.Monitoring.Metrics.ElasticsearchRefs
}

func (es *Elasticsearch) GetMonitoringLogsRefs() []commonv1.ObjectSelector {
	return es.Spec.Monitoring.Logs.ElasticsearchRefs
}

func (es *Elasticsearch) MonitoringAssociation(ref commonv1.ObjectSelector) commonv1.Association {
	return &EsMonitoringAssociation{
		Elasticsearch: es,
		ref:           ref.WithDefaultNamespace(es.Namespace).NamespacedName(),
	}
}

// DisabledPredicates returns the set of predicates that are currently disabled by the
// DisableUpgradePredicatesAnnotation annotation.
func (es Elasticsearch) DisabledPredicates() set.StringSet {
	return setFromAnnotations(DisableUpgradePredicatesAnnotation, es.Annotations)
}
