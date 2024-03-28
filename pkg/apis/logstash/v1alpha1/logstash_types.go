// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
)

type LogstashHealth string

const (
	LogstashContainerName = "logstash"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Logstash"

	// LogstashRedHealth means that the health is neither yellow nor green.
	LogstashRedHealth LogstashHealth = "red"

	// LogstashYellowHealth means that:
	// 1) at least one Pod is Ready, and
	// 2) any associations are configured and established
	LogstashYellowHealth LogstashHealth = "yellow"

	// LogstashGreenHealth means that:
	// 1) all Pods are Ready, and
	// 2) any associations are configured and established
	LogstashGreenHealth LogstashHealth = "green"
)

// LogstashSpec defines the desired state of Logstash
type LogstashSpec struct {
	// Version of the Logstash.
	Version string `json:"version"`

	Count int32 `json:"count,omitempty"`

	// Image is the Logstash Docker image to deploy. Version and Type have to match the Logstash in the image.
	// +kubebuilder:validation:Optional
	Image string `json:"image,omitempty"`

	// ElasticsearchRefs are references to Elasticsearch clusters running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	ElasticsearchRefs []ElasticsearchCluster `json:"elasticsearchRefs,omitempty"`

	// Config holds the Logstash configuration. At most one of [`Config`, `ConfigRef`] can be specified.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`

	// ConfigRef contains a reference to an existing Kubernetes Secret holding the Logstash configuration.
	// Logstash settings must be specified as yaml, under a single "logstash.yml" entry. At most one of [`Config`, `ConfigRef`]
	// can be specified.
	// +kubebuilder:validation:Optional
	ConfigRef *commonv1.ConfigSource `json:"configRef,omitempty"`

	// Pipelines holds the Logstash Pipelines. At most one of [`Pipelines`, `PipelinesRef`] can be specified.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Pipelines []commonv1.Config `json:"pipelines,omitempty"`

	// PipelinesRef contains a reference to an existing Kubernetes Secret holding the Logstash Pipelines.
	// Logstash pipelines must be specified as yaml, under a single "pipelines.yml" entry. At most one of [`Pipelines`, `PipelinesRef`]
	// can be specified.
	// +kubebuilder:validation:Optional
	PipelinesRef *commonv1.ConfigSource `json:"pipelinesRef,omitempty"`

	// Services contains details of services that Logstash should expose - similar to the HTTP layer configuration for the
	// rest of the stack, but also applicable for more use cases than the metrics API, as logstash may need to
	// be opened up for other services: Beats, TCP, UDP, etc, inputs.
	// +kubebuilder:validation:Optional
	Services []LogstashService `json:"services,omitempty"`

	// Monitoring enables you to collect and ship log and monitoring data of this Logstash.
	// Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different
	// Elasticsearch monitoring clusters running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	Monitoring commonv1.Monitoring `json:"monitoring,omitempty"`

	// PodTemplate provides customisation options for the Logstash pods.
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying StatefulSet.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Logstash.
	// Secrets data can be then referenced in the Logstash config using the Secret's keys or as specified in `Entries` field of
	// each SecureSetting.
	// +kubebuilder:validation:Optional
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// UpdateStrategy is a StatefulSetUpdateStrategy. The default type is "RollingUpdate".
	// +kubebuilder:validation:Optional
	UpdateStrategy appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod.
	// Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.
	// Items defined here take precedence over any default claims added by the operator with the same name.
	// +kubebuilder:validation:Optional
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

type LogstashService struct {
	Name string `json:"name,omitempty"`
	// Service defines the template for the associated Kubernetes Service object.
	Service commonv1.ServiceTemplate `json:"service,omitempty"`
	// TLS defines options for configuring TLS for HTTP.
	TLS commonv1.TLSOptions `json:"tls,omitempty"`
}

// ElasticsearchCluster is a named reference to an Elasticsearch cluster which can be used in a Logstash pipeline.
type ElasticsearchCluster struct {
	commonv1.ObjectSelector `json:",omitempty,inline"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// ClusterName is an alias for the cluster to be used to refer to the Elasticsearch cluster in Logstash
	// configuration files, and will be used to identify "named clusters" in Logstash
	ClusterName string `json:"clusterName,omitempty"`
}

// LogstashStatus defines the observed state of Logstash
type LogstashStatus struct {
	// Version of the stack resource currently running. During version upgrades, multiple versions may run
	// in parallel: this value specifies the lowest version currently running.
	Version string `json:"version,omitempty"`

	// +kubebuilder:validation:Optional
	ExpectedNodes int32 `json:"expectedNodes,omitempty"`
	// +kubebuilder:validation:Optional
	AvailableNodes int32 `json:"availableNodes,omitempty"`

	// +kubebuilder:validation:Optional
	Health LogstashHealth `json:"health,omitempty"`

	// ObservedGeneration is the most recent generation observed for this Logstash instance.
	// It corresponds to the metadata generation, which is updated on mutation by the API Server.
	// If the generation observed in status diverges from the generation in metadata, the Logstash
	// controller has not yet processed the changes contained in the Logstash specification.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ElasticsearchAssociationStatus is the status of any auto-linking to Elasticsearch clusters.
	ElasticsearchAssociationsStatus commonv1.AssociationStatusMap `json:"elasticsearchAssociationsStatus,omitempty"`

	// MonitoringAssociationStatus is the status of any auto-linking to monitoring Elasticsearch clusters.
	MonitoringAssociationStatus commonv1.AssociationStatusMap `json:"monitoringAssociationStatus,omitempty"`

	Selector string `json:"selector"`
}

// +kubebuilder:object:root=true

// Logstash is the Schema for the logstashes API
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories=elastic,shortName=ls
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health",description="Health"
// +kubebuilder:printcolumn:name="available",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="expected",type="integer",JSONPath=".status.expectedNodes",description="Expected nodes"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Logstash version"
// +kubebuilder:subresource:scale:specpath=.spec.count,statuspath=.status.expectedNodes,selectorpath=.status.selector
// +kubebuilder:storageversion
type Logstash struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec                 LogstashSpec                                         `json:"spec,omitempty"`
	Status               LogstashStatus                                       `json:"status,omitempty"`
	EsAssocConfs         map[commonv1.ObjectSelector]commonv1.AssociationConf `json:"-"`
	MonitoringAssocConfs map[commonv1.ObjectSelector]commonv1.AssociationConf `json:"-"`
}

// +kubebuilder:object:root=true

// LogstashList contains a list of Logstash
type LogstashList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Logstash `json:"items"`
}

func (l *Logstash) ElasticsearchRefs() []commonv1.ObjectSelector {
	refs := make([]commonv1.ObjectSelector, len(l.Spec.ElasticsearchRefs))
	for i, r := range l.Spec.ElasticsearchRefs {
		refs[i] = r.ObjectSelector
	}
	return refs
}
func (l *Logstash) ServiceAccountName() string {
	return l.Spec.ServiceAccountName
}

func (l *Logstash) SecureSettings() []commonv1.SecretSource {
	return l.Spec.SecureSettings
}

// IsMarkedForDeletion returns true if the Logstash is going to be deleted
func (l *Logstash) IsMarkedForDeletion() bool {
	return !l.DeletionTimestamp.IsZero()
}

// GetObservedGeneration will return the observedGeneration from the Elastic Logstash's status.
func (l *Logstash) GetObservedGeneration() int64 {
	return l.Status.ObservedGeneration
}

func (l *Logstash) GetAssociations() []commonv1.Association {
	associations := make(
		[]commonv1.Association,
		0,
		len(l.Spec.ElasticsearchRefs)+len(l.Spec.Monitoring.Metrics.ElasticsearchRefs)+len(l.Spec.Monitoring.Logs.ElasticsearchRefs),
	)

	for _, ref := range l.Spec.ElasticsearchRefs {
		associations = append(associations, &LogstashESAssociation{
			Logstash: l,
			ElasticsearchCluster: ElasticsearchCluster{
				ObjectSelector: ref.WithDefaultNamespace(l.Namespace),
				ClusterName:    ref.ClusterName,
			},
		})
	}

	for _, ref := range l.Spec.Monitoring.Metrics.ElasticsearchRefs {
		if ref.IsDefined() {
			associations = append(associations, &LogstashMonitoringAssociation{
				Logstash: l,
				ref:      ref.WithDefaultNamespace(l.Namespace),
			})
		}
	}
	for _, ref := range l.Spec.Monitoring.Logs.ElasticsearchRefs {
		if ref.IsDefined() {
			associations = append(associations, &LogstashMonitoringAssociation{
				Logstash: l,
				ref:      ref.WithDefaultNamespace(l.Namespace),
			})
		}
	}

	return associations
}

func (l *Logstash) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		if len(l.Spec.ElasticsearchRefs) > 0 {
			return l.Status.ElasticsearchAssociationsStatus
		}
	case commonv1.LogstashMonitoringAssociationType:
		for _, esRef := range l.Spec.Monitoring.Metrics.ElasticsearchRefs {
			if esRef.IsDefined() {
				return l.Status.MonitoringAssociationStatus
			}
		}
		for _, esRef := range l.Spec.Monitoring.Logs.ElasticsearchRefs {
			if esRef.IsDefined() {
				return l.Status.MonitoringAssociationStatus
			}
		}
	}

	return commonv1.AssociationStatusMap{}
}

func (l *Logstash) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		l.Status.ElasticsearchAssociationsStatus = status
		return nil
	case commonv1.LogstashMonitoringAssociationType:
		l.Status.MonitoringAssociationStatus = status
		return nil
	default:
		return fmt.Errorf("association type %s not known", typ)
	}
}

type LogstashESAssociation struct {
	// The associated Logstash
	*Logstash
	ElasticsearchCluster
}

var _ commonv1.Association = &LogstashESAssociation{}

func (lses *LogstashESAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (lses *LogstashESAssociation) Associated() commonv1.Associated {
	if lses == nil {
		return nil
	}
	if lses.Logstash == nil {
		lses.Logstash = &Logstash{}
	}
	return lses.Logstash
}

func (lses *LogstashESAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (lses *LogstashESAssociation) AssociationRef() commonv1.ObjectSelector {
	return lses.ElasticsearchCluster.ObjectSelector
}

func (lses *LogstashESAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName(lses.ElasticsearchCluster.ObjectSelector)
}

func (lses *LogstashESAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConfByRef(lses, lses.ElasticsearchCluster.ObjectSelector, lses.EsAssocConfs)
}

func (lses *LogstashESAssociation) SetAssociationConf(conf *commonv1.AssociationConf) {
	if lses.EsAssocConfs == nil {
		lses.EsAssocConfs = make(map[commonv1.ObjectSelector]commonv1.AssociationConf)
	}
	if conf != nil {
		lses.EsAssocConfs[lses.ElasticsearchCluster.ObjectSelector] = *conf
	}
}

func (lses *LogstashESAssociation) SupportsAuthAPIKey() bool {
	return false
}

func (lses *LogstashESAssociation) AssociationID() string {
	return fmt.Sprintf("%s-%s", lses.ElasticsearchCluster.ObjectSelector.Namespace, lses.ElasticsearchCluster.ObjectSelector.NameOrSecretName())
}

type LogstashMonitoringAssociation struct {
	// The associated Logstash
	*Logstash
	// ref is the object selector of the monitoring Elasticsearch referenced in the Association
	ref commonv1.ObjectSelector
}

var _ commonv1.Association = &LogstashMonitoringAssociation{}

func (lsmon *LogstashMonitoringAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (lsmon *LogstashMonitoringAssociation) Associated() commonv1.Associated {
	if lsmon == nil {
		return nil
	}
	if lsmon.Logstash == nil {
		lsmon.Logstash = &Logstash{}
	}
	return lsmon.Logstash
}

func (lsmon *LogstashMonitoringAssociation) AssociationConfAnnotationName() string {
	// Use a custom suffix for monitoring elasticsearchRefs to avoid clashes with other elasticsearchRefs
	return commonv1.FormatNameWithID(commonv1.ElasticsearchConfigAnnotationNameBase+"%s-sm", hash.HashObject(lsmon.ref))
}

func (lsmon *LogstashMonitoringAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.LogstashMonitoringAssociationType
}

func (lsmon *LogstashMonitoringAssociation) AssociationRef() commonv1.ObjectSelector {
	return lsmon.ref
}

func (lsmon *LogstashMonitoringAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConfByRef(lsmon, lsmon.ref, lsmon.MonitoringAssocConfs)
}

func (lsmon *LogstashMonitoringAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	if lsmon.MonitoringAssocConfs == nil {
		lsmon.MonitoringAssocConfs = make(map[commonv1.ObjectSelector]commonv1.AssociationConf)
	}
	if assocConf != nil {
		lsmon.MonitoringAssocConfs[lsmon.ref] = *assocConf
	}
}

func (lsmon *LogstashMonitoringAssociation) SupportsAuthAPIKey() bool {
	return false
}

func (lsmon *LogstashMonitoringAssociation) AssociationID() string {
	return lsmon.ref.ToID()
}

func (l *Logstash) GetMonitoringMetricsRefs() []commonv1.ObjectSelector {
	return l.Spec.Monitoring.Metrics.ElasticsearchRefs
}

func (l *Logstash) GetMonitoringLogsRefs() []commonv1.ObjectSelector {
	return l.Spec.Monitoring.Logs.ElasticsearchRefs
}

func (l *Logstash) MonitoringAssociation(esRef commonv1.ObjectSelector) commonv1.Association {
	return &LogstashMonitoringAssociation{
		Logstash: l,
		ref:      esRef.WithDefaultNamespace(l.Namespace),
	}
}

// APIServerService returns the user defined API Service
func (l *Logstash) APIServerService() LogstashService {
	for _, service := range l.Spec.Services {
		if UserServiceName(l.Name, service.Name) == APIServiceName(l.Name) {
			return service
		}
	}
	return LogstashService{}
}

// APIServerTLSOptions returns the user defined TLSOptions of API Service
func (l *Logstash) APIServerTLSOptions() commonv1.TLSOptions {
	return l.APIServerService().TLS
}

func init() {
	SchemeBuilder.Register(&Logstash{}, &LogstashList{})
}
