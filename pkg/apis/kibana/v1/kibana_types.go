// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	"github.com/blang/semver/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

const (
	KibanaContainerName = "kibana"
	// Kind is inferred from the struct name using reflection in SchemeBuilder.Register()
	// we duplicate it as a constant here for practical purposes.
	Kind = "Kibana"
	// KibanaServiceAccount is the Elasticsearch service account to be used to authenticate.
	KibanaServiceAccount commonv1.ServiceAccountName = "kibana"
)

// +kubebuilder:object:root=true

// Kibana represents a Kibana resource in a Kubernetes cluster.
// +kubebuilder:resource:categories=elastic,shortName=kb
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="health",type="string",JSONPath=".status.health"
// +kubebuilder:printcolumn:name="nodes",type="integer",JSONPath=".status.availableNodes",description="Available nodes"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Kibana version"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:scale:specpath=.spec.count,statuspath=.status.count,selectorpath=.status.selector
// +kubebuilder:storageversion
type Kibana struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KibanaSpec   `json:"spec,omitempty"`
	Status KibanaStatus `json:"status,omitempty"`
	// assocConf holds the configuration for the Elasticsearch association
	assocConf *commonv1.AssociationConf `json:"-"`
	// entAssocConf holds the configuration for the Enterprise Search association
	entAssocConf *commonv1.AssociationConf `json:"-"`
	// monitoringAssocConf holds the configuration for the monitoring Elasticsearch clusters association
	monitoringAssocConfs map[types.NamespacedName]commonv1.AssociationConf `json:"-"`
}

// +kubebuilder:object:root=true

// KibanaList contains a list of Kibana
type KibanaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kibana `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kibana{}, &KibanaList{})
}

// KibanaSpec holds the specification of a Kibana instance.
type KibanaSpec struct {
	// Version of Kibana.
	Version string `json:"version"`

	// Image is the Kibana Docker image to deploy.
	Image string `json:"image,omitempty"`

	// Count of Kibana instances to deploy.
	Count int32 `json:"count,omitempty"`

	// ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.
	ElasticsearchRef commonv1.ObjectSelector `json:"elasticsearchRef,omitempty"`

	// EnterpriseSearchRef is a reference to an EnterpriseSearch running in the same Kubernetes cluster.
	// Kibana provides the default Enterprise Search UI starting version 7.14.
	EnterpriseSearchRef commonv1.ObjectSelector `json:"enterpriseSearchRef,omitempty"`

	// Config holds the Kibana configuration. See: https://www.elastic.co/guide/en/kibana/current/settings.html
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`

	// HTTP holds the HTTP layer configuration for Kibana.
	HTTP commonv1.HTTPConfig `json:"http,omitempty"`

	// PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Kibana pods
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Kibana.
	SecureSettings []commonv1.SecretSource `json:"secureSettings,omitempty"`

	// ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.
	// Can only be used if ECK is enforcing RBAC on references.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Monitoring enables you to collect and ship log and monitoring data of this Kibana.
	// See https://www.elastic.co/guide/en/kibana/current/xpack-monitoring.html.
	// Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different
	// Elasticsearch monitoring clusters running in the same Kubernetes cluster.
	// +kubebuilder:validation:Optional
	Monitoring Monitoring `json:"monitoring,omitempty"`
}

type Monitoring struct {
	// Metrics holds references to Elasticsearch clusters which will receive monitoring data from this Kibana.
	// +kubebuilder:validation:Optional
	Metrics MetricsMonitoring `json:"metrics,omitempty"`
	// Logs holds references to Elasticsearch clusters which will receive log data from this Kibana.
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

// KibanaStatus defines the observed state of Kibana
type KibanaStatus struct {
	commonv1.DeploymentStatus `json:",inline"`

	// AssociationStatus is the status of any auto-linking to Elasticsearch clusters.
	// This field is deprecated and will be removed in a future release. Use ElasticsearchAssociationStatus instead.
	AssociationStatus commonv1.AssociationStatus `json:"associationStatus,omitempty"`

	// ElasticsearchAssociationStatus is the status of any auto-linking to Elasticsearch clusters.
	ElasticsearchAssociationStatus commonv1.AssociationStatus `json:"elasticsearchAssociationStatus,omitempty"`

	// EnterpriseSearchAssociationStatus is the status of any auto-linking to Enterprise Search.
	EnterpriseSearchAssociationStatus commonv1.AssociationStatus `json:"enterpriseSearchAssociationStatus,omitempty"`

	// MonitoringAssociationStatus is the status of any auto-linking to monitoring Elasticsearch clusters.
	MonitoringAssociationStatus commonv1.AssociationStatusMap `json:"monitoringAssociationStatus,omitempty"`

	// ObservedGeneration is the most recent generation observed for this Kibana instance.
	// It corresponds to the metadata generation, which is updated on mutation by the API Server.
	// If the generation observed in status diverges from the generation in metadata, the Kibana
	// controller has not yet processed the changes contained in the Kibana specification.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// IsMarkedForDeletion returns true if the Kibana is going to be deleted
func (k *Kibana) IsMarkedForDeletion() bool {
	return !k.DeletionTimestamp.IsZero()
}

func (k *Kibana) SecureSettings() []commonv1.SecretSource {
	return k.Spec.SecureSettings
}

func (k *Kibana) ServiceAccountName() string {
	return k.Spec.ServiceAccountName
}

var KibanaServiceAccountMinVersion = semver.MustParse("7.17.0")

// -- associations

var _ commonv1.Associated = &Kibana{}

func (k *Kibana) Associated() commonv1.Associated {
	return k
}

func (k *Kibana) GetAssociations() []commonv1.Association {
	associations := make([]commonv1.Association, 0)

	if k.Spec.ElasticsearchRef.IsDefined() {
		associations = append(associations, &KibanaEsAssociation{
			Kibana: k,
		})
	}
	if k.Spec.EnterpriseSearchRef.IsDefined() {
		associations = append(associations, &KibanaEntAssociation{
			Kibana: k,
		})
	}
	for _, ref := range k.Spec.Monitoring.Metrics.ElasticsearchRefs {
		if ref.IsDefined() {
			associations = append(associations, &KbMonitoringAssociation{
				Kibana: k,
				ref:    ref.WithDefaultNamespace(k.Namespace).NamespacedName(),
			})
		}
	}
	for _, ref := range k.Spec.Monitoring.Logs.ElasticsearchRefs {
		if ref.IsDefined() {
			associations = append(associations, &KbMonitoringAssociation{
				Kibana: k,
				ref:    ref.WithDefaultNamespace(k.Namespace).NamespacedName(),
			})
		}
	}

	return associations
}

func (k *Kibana) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		if k.Spec.ElasticsearchRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(k.Status.ElasticsearchAssociationStatus)
		}
	case commonv1.EntAssociationType:
		if k.Spec.EnterpriseSearchRef.IsDefined() {
			return commonv1.NewSingleAssociationStatusMap(k.Status.EnterpriseSearchAssociationStatus)
		}
	case commonv1.KbMonitoringAssociationType:
		for _, esRef := range k.Spec.Monitoring.Metrics.ElasticsearchRefs {
			if esRef.IsDefined() {
				return k.Status.MonitoringAssociationStatus
			}
		}
		for _, esRef := range k.Spec.Monitoring.Logs.ElasticsearchRefs {
			if esRef.IsDefined() {
				return k.Status.MonitoringAssociationStatus
			}
		}
	}

	return commonv1.AssociationStatusMap{}
}

func (k *Kibana) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	switch typ {
	case commonv1.ElasticsearchAssociationType:
		single, err := status.Single()
		if err != nil {
			return err
		}
		k.Status.ElasticsearchAssociationStatus = single
		// also set Status.AssociationStatus to report the status of the association with es,
		// for backward compatibility reasons
		k.Status.AssociationStatus = single
		return nil
	case commonv1.EntAssociationType:
		single, err := status.Single()
		if err != nil {
			return err
		}
		k.Status.EnterpriseSearchAssociationStatus = single
		return nil
	case commonv1.KbMonitoringAssociationType:
		k.Status.MonitoringAssociationStatus = status
		return nil
	default:
		return fmt.Errorf("association type %s not known", typ)
	}
}

// GetObservedGeneration will return the observed generation from the Kibana status.
func (k *Kibana) GetObservedGeneration() int64 {
	return k.Status.ObservedGeneration
}

// -- association with Elasticsearch

func (k *Kibana) EsAssociation() *KibanaEsAssociation {
	return &KibanaEsAssociation{Kibana: k}
}

// KibanaEsAssociation helps to manage the Kibana / Elasticsearch association.
type KibanaEsAssociation struct {
	*Kibana
}

var _ commonv1.Association = &KibanaEsAssociation{}

func (kbes *KibanaEsAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	v, err := version.Parse(kbes.Spec.Version)
	if err != nil {
		return "", err
	}
	if v.GTE(KibanaServiceAccountMinVersion) {
		return KibanaServiceAccount, nil
	}
	return "", nil
}

func (kbes *KibanaEsAssociation) Associated() commonv1.Associated {
	if kbes == nil {
		return nil
	}
	if kbes.Kibana == nil {
		kbes.Kibana = &Kibana{}
	}
	return kbes.Kibana
}

func (kbes *KibanaEsAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationNameBase
}

func (kbes *KibanaEsAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.ElasticsearchAssociationType
}

func (kbes *KibanaEsAssociation) AssociationRef() commonv1.ObjectSelector {
	return kbes.Spec.ElasticsearchRef.WithDefaultNamespace(kbes.Namespace)
}

func (kbes *KibanaEsAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(kbes, kbes.assocConf)
}

func (kbes *KibanaEsAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	kbes.assocConf = assocConf
}

func (kbes *KibanaEsAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
}

// -- association with Enterprise Search

func (k *Kibana) EntAssociation() *KibanaEntAssociation {
	return &KibanaEntAssociation{Kibana: k}
}

// KibanaEntAssociation helps to manage the Kibana / Enterprise Search association.
type KibanaEntAssociation struct {
	*Kibana
}

var _ commonv1.Association = &KibanaEntAssociation{}

func (kbent *KibanaEntAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (kbent *KibanaEntAssociation) Associated() commonv1.Associated {
	if kbent == nil {
		return nil
	}
	if kbent.Kibana == nil {
		kbent.Kibana = &Kibana{}
	}
	return kbent.Kibana
}

func (kbent *KibanaEntAssociation) AssociationConfAnnotationName() string {
	return commonv1.EntConfigAnnotationNameBase
}

func (kbent *KibanaEntAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.EntAssociationType
}

func (kbent *KibanaEntAssociation) AssociationRef() commonv1.ObjectSelector {
	return kbent.Spec.EnterpriseSearchRef.WithDefaultNamespace(kbent.Namespace)
}

func (kbent *KibanaEntAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConf(kbent, kbent.entAssocConf)
}

func (kbent *KibanaEntAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	kbent.entAssocConf = assocConf
}

func (kbent *KibanaEntAssociation) AssociationID() string {
	return commonv1.SingletonAssociationID
}

// -- association with monitoring Elasticsearch clusters

// KbMonitoringAssociation helps to manage the Kibana / monitoring Elasticsearch clusters association.
type KbMonitoringAssociation struct {
	// The associated Kibana
	*Kibana
	// ref is the namespaced name of the monitoring Elasticsearch referenced in the Association
	ref types.NamespacedName
}

var _ commonv1.Association = &KbMonitoringAssociation{}

func (kbmon *KbMonitoringAssociation) ElasticServiceAccount() (commonv1.ServiceAccountName, error) {
	return "", nil
}

func (kbmon *KbMonitoringAssociation) Associated() commonv1.Associated {
	if kbmon == nil {
		return nil
	}
	if kbmon.Kibana == nil {
		kbmon.Kibana = &Kibana{}
	}
	return kbmon.Kibana
}

func (kbmon *KbMonitoringAssociation) AssociationConfAnnotationName() string {
	return commonv1.ElasticsearchConfigAnnotationName(kbmon.ref)
}

func (kbmon *KbMonitoringAssociation) AssociationType() commonv1.AssociationType {
	return commonv1.KbMonitoringAssociationType
}

func (kbmon *KbMonitoringAssociation) AssociationRef() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      kbmon.ref.Name,
		Namespace: kbmon.ref.Namespace,
	}
}

func (kbmon *KbMonitoringAssociation) AssociationConf() (*commonv1.AssociationConf, error) {
	return commonv1.GetAndSetAssociationConfByRef(kbmon, kbmon.ref, kbmon.monitoringAssocConfs)
}

func (kbmon *KbMonitoringAssociation) SetAssociationConf(assocConf *commonv1.AssociationConf) {
	if kbmon.monitoringAssocConfs == nil {
		kbmon.monitoringAssocConfs = make(map[types.NamespacedName]commonv1.AssociationConf)
	}
	if assocConf != nil {
		kbmon.monitoringAssocConfs[kbmon.ref] = *assocConf
	}
}

func (kbmon *KbMonitoringAssociation) AssociationID() string {
	return fmt.Sprintf("%s-%s", kbmon.ref.Namespace, kbmon.ref.Name)
}

// -- HasMonitoring methods

func (k *Kibana) GetMonitoringMetricsRefs() []commonv1.ObjectSelector {
	return k.Spec.Monitoring.Metrics.ElasticsearchRefs
}

func (k *Kibana) GetMonitoringLogsRefs() []commonv1.ObjectSelector {
	return k.Spec.Monitoring.Logs.ElasticsearchRefs
}

func (k *Kibana) MonitoringAssociation(esRef commonv1.ObjectSelector) commonv1.Association {
	return &KbMonitoringAssociation{
		Kibana: k,
		ref:    esRef.WithDefaultNamespace(k.Namespace).NamespacedName(),
	}
}
