// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	singleStatusKey = ""
)

// AssociationType is the type of an association resource, eg. for Kibana-ES association, AssociationType identifies ES.
type AssociationType string

// AssociationStatus is the status of an association resource.
type AssociationStatus string

// AssociationStatusMap is the map of association's namespaced name string to its AssociationStatus. For resources that
// have a single Association of a given type (for ex. single ES reference), this map contains a single entry.
type AssociationStatusMap map[string]AssociationStatus

// NewSingleAssociationStatusMap creates an AssociationStatusMap that expects only a single Association. Using a
// well-known key allows to keep serialization of the map backwards compatible.
func NewSingleAssociationStatusMap(status AssociationStatus) AssociationStatusMap {
	return map[string]AssociationStatus{
		singleStatusKey: status,
	}
}

func (asm AssociationStatusMap) String() string {
	// check if it's single status map and return only status string if yes
	// this allows to keep serialization backwards compatible
	if len(asm) == 1 {
		for key, value := range asm {
			if key == singleStatusKey {
				return string(value)
			}
		}
	}

	// sort by keys to make String() stable
	keys := make([]string, 0, len(asm))
	for key := range asm {
		keys = append(keys, key)
	}
	sort.StringSlice(keys).Sort()

	var i int
	var sb strings.Builder
	for _, key := range keys {
		value := asm[key]
		i++
		sb.WriteString(key + ": " + string(value))
		if len(asm) != i {
			sb.WriteString(", ")
		}
	}
	return sb.String()
}

func (asm AssociationStatusMap) Single() (AssociationStatus, error) {
	if len(asm) > 1 {
		return "", fmt.Errorf("expected at most one AssociationStatus in map, but found %d: %s", len(asm), asm)
	}

	// returns the only AssociationStatus present or zero value if none are
	var result AssociationStatus
	for _, status := range asm {
		result = status
	}
	return result, nil
}

// AllEstablished returns true iff all Associations have AssociationEstablished status, false otherwise.
func (asm AssociationStatusMap) AllEstablished() bool {
	for _, status := range asm {
		if status != AssociationEstablished {
			return false
		}
	}
	return true
}

const (
	ElasticsearchConfigAnnotationNameBase = "association.k8s.elastic.co/es-conf"
	ElasticsearchAssociationType          = "elasticsearch"
	EsMonitoringAssociationType           = "es-monitoring"

	KibanaConfigAnnotationNameBase = "association.k8s.elastic.co/kb-conf"
	KibanaAssociationType          = "kibana"
	KbMonitoringAssociationType    = "kb-monitoring"

	EntConfigAnnotationNameBase = "association.k8s.elastic.co/ent-conf"
	EntAssociationType          = "ent"

	FleetServerConfigAnnotationNameBase = "association.k8s.elastic.co/fs-conf"
	FleetServerAssociationType          = "fleetserver"

	AssociationUnknown     AssociationStatus = ""
	AssociationPending     AssociationStatus = "Pending"
	AssociationEstablished AssociationStatus = "Established"
	AssociationFailed      AssociationStatus = "Failed"

	// SingletonAssociationID is an `AssociationID` used for Associations for resources that can have only a single
	// Association of each type. For example, Kibana can only have a single ES Association, so Kibana-ES Associations
	// should use `SingletonAssociationID` as their `AssociationID`. On the contrary, Agent can have unbounded number
	// of Associations so Agent-ES Associations should _not_ use `SingletonAssociationID`.
	SingletonAssociationID = ""

	// NoAuthRequiredValue is the value set for AuthSecretName if no authentication
	// credentials are necessary for that association.
	NoAuthRequiredValue = "-"
)

type ServiceAccountName string

// Associated represents an Elastic stack resource that is associated with other stack resources.
// Examples:
// - Kibana can be associated with Elasticsearch
// - APMServer can be associated with Elasticsearch and Kibana
// - EnterpriseSearch can be associated with Elasticsearch
// - Beat can be associated with Elasticsearch and Kibana
// - Agent can be associated with multiple Elasticsearches
// +kubebuilder:object:generate=false
type Associated interface {
	metav1.Object
	runtime.Object
	ServiceAccountName() string
	GetAssociations() []Association
	AssociationStatusMap(typ AssociationType) AssociationStatusMap
	SetAssociationStatusMap(typ AssociationType, statusMap AssociationStatusMap) error
}

// Association interface helps to manage the Spec fields involved in an association.
// +kubebuilder:object:generate=false
type Association interface {
	Associated

	// ElasticServiceAccount returns the Elasticsearch service account name to be used for authentication.
	ElasticServiceAccount() (ServiceAccountName, error)

	// Associated can be used to retrieve the associated object
	Associated() Associated

	// AssociationType returns a string describing the type of the target resource (elasticsearch most of the time)
	// It is mostly used to build some other strings depending on the type of the targeted resource.
	AssociationType() AssociationType

	// AssociationRef is a reference to the associated resource. If defined with a Name then the Namespace is expected
	// to be set in the returned object.
	AssociationRef() ObjectSelector

	// AssociationConfAnnotationName is the name of the annotation used to define the config for the associated resource.
	// It is used by the association controller to store the configuration and by the controller which is
	// managing the associated resource to build the appropriate configuration.
	AssociationConfAnnotationName() string

	// AssociationConf is the configuration of the Association allowing to connect to the Association resource.
	AssociationConf() (*AssociationConf, error)
	SetAssociationConf(*AssociationConf)

	// AssociationID uniquely identifies this Association among all Associations of the same type belonging to Associated()
	AssociationID() string
}

// FormatNameWithID conditionally formats `template`. `template` is expected to have a single %s verb.
// If `id` is empty, the %s verb will be formatted with empty string. Otherwise %s verb will be replaced with `-id`.
// Eg:
// FormatNameWithID("name%s", "") returns "name"
// FormatNameWithID("name%s", "ns1-es1") returns "name-ns1-es1"
// FormatNameWithID("name%s", "ns2-es2") returns "name-ns2-es2"
// This function exists to abstract this conditional logic away from the callers. It can be used to format names
// for objects differing only by id, that would otherwise collide. In addition, it allows to preserve current naming
// for object types with a single association and introduce object types with unbounded number of associations.
func FormatNameWithID(template string, id string) string {
	if id != SingletonAssociationID {
		// we want names to be changed for any id but SingletonAssociationID
		id = fmt.Sprintf("-%s", id)
	}

	return fmt.Sprintf(template, id)
}

// AssociationConf holds the association configuration of a referenced resource in an association.
type AssociationConf struct {
	AuthSecretName   string `json:"authSecretName"`
	AuthSecretKey    string `json:"authSecretKey"`
	IsServiceAccount bool   `json:"isServiceAccount"`
	CACertProvided   bool   `json:"caCertProvided"`
	CASecretName     string `json:"caSecretName"`
	URL              string `json:"url"`
	// Version of the referenced resource. If a version upgrade is in progress,
	// matches the lowest running version. May be empty if unknown.
	Version string `json:"version"`
}

// IsConfigured returns true if all the fields are set.
func (ac *AssociationConf) IsConfigured() bool {
	if ac.GetCACertProvided() && !ac.CAIsConfigured() {
		return false
	}

	return ac.AuthIsConfigured() && ac.URLIsConfigured()
}

// AuthIsConfigured returns true if the auth fields are set.
func (ac *AssociationConf) AuthIsConfigured() bool {
	if ac == nil {
		return false
	}
	if ac.NoAuthRequired() {
		// auth fields are not required, but still configured
		return true
	}
	// ensure both secret name and secret key are provided
	return ac.AuthSecretName != "" && ac.AuthSecretKey != ""
}

func (ac *AssociationConf) NoAuthRequired() bool {
	return ac.AuthSecretName == NoAuthRequiredValue
}

// CAIsConfigured returns true if the CA field is set.
func (ac *AssociationConf) CAIsConfigured() bool {
	if ac == nil {
		return false
	}
	return ac.CASecretName != ""
}

// URLIsConfigured returns true if the URL field is set.
func (ac *AssociationConf) URLIsConfigured() bool {
	if ac == nil {
		return false
	}
	return ac.URL != ""
}

func (ac *AssociationConf) GetAuthSecretName() string {
	if ac == nil {
		return ""
	}
	return ac.AuthSecretName
}

func (ac *AssociationConf) GetAuthSecretKey() string {
	if ac == nil {
		return ""
	}
	return ac.AuthSecretKey
}

func (ac *AssociationConf) GetCACertProvided() bool {
	if ac == nil {
		return false
	}
	return ac.CACertProvided
}

func (ac *AssociationConf) GetCASecretName() string {
	if ac == nil {
		return ""
	}
	return ac.CASecretName
}

func (ac *AssociationConf) GetURL() string {
	if ac == nil {
		return ""
	}
	return ac.URL
}

func (ac *AssociationConf) GetVersion() string {
	if ac == nil {
		return ""
	}
	return ac.Version
}

func ElasticsearchConfigAnnotationName(esNsn types.NamespacedName) string {
	// annotation key should be stable to allow the Elasticsearch Controller to only pick up the ones it expects,
	// based on ElasticsearchRefs

	nsNameHash := fnv.New32a()
	// concat with dot to avoid collisions, as namespace can't contain dots
	_, _ = nsNameHash.Write([]byte(fmt.Sprintf("%s.%s", esNsn.Namespace, esNsn.Name)))
	hash := fmt.Sprint(nsNameHash.Sum32())

	return FormatNameWithID(
		ElasticsearchConfigAnnotationNameBase+"%s",
		hash,
	)
}
