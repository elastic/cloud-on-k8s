// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AssociationType is the type of an association resource, eg. for Kibana-ES association, AssociationType identifies ES.
type AssociationType string

// AssociationStatus is the status of an association resource.
type AssociationStatus string

// AssociationStatusMap is the map of association's namespaced name string to its AssociationStatus
type AssociationStatusMap map[string]AssociationStatus

func NewAssociationStatusMap(selector ObjectSelector, status AssociationStatus) AssociationStatusMap {
	return map[string]AssociationStatus{
		selector.NamespacedName().String(): status,
	}
}

func (asm AssociationStatusMap) String() string {
	var i int
	var sb strings.Builder
	for key, value := range asm {
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

func (asm AssociationStatusMap) Aggregate() AssociationStatus {
	worst := AssociationUnknown
	for _, status := range asm {
		switch status {
		case AssociationEstablished:
			if worst == AssociationUnknown {
				worst = AssociationEstablished
			}
		case AssociationPending:
			if worst == AssociationUnknown || worst == AssociationEstablished {
				worst = AssociationPending
			}
		case AssociationFailed:
			return AssociationFailed
		}
	}

	return worst
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

	KibanaConfigAnnotationNameBase = "association.k8s.elastic.co/kb-conf"
	KibanaAssociationType          = "kibana"

	AssociationUnknown     AssociationStatus = ""
	AssociationPending     AssociationStatus = "Pending"
	AssociationEstablished AssociationStatus = "Established"
	AssociationFailed      AssociationStatus = "Failed"
)

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

	// Associated can be used to retrieve the associated object
	Associated() Associated

	// AssociationType returns a string describing the type of the target resource (elasticsearch most of the time)
	// It is mostly used to build some other strings depending on the type of the targeted resource.
	AssociationType() AssociationType

	// Reference to the associated resource. If defined with a Name then the Namespace is expected to be set in the returned object.
	AssociationRef() ObjectSelector

	// AnnotationName is the name of the annotation used to define the config for the associated resource.
	// It is used by the association controller to store the configuration and by the controller which is
	// managing the associated resource to build the appropriate configuration.
	AnnotationName() string

	// Configuration
	AssociationConf() *AssociationConf
	SetAssociationConf(*AssociationConf)

	// ID uniquely identifies this Association among all Associations of the same type belonging to Associated()
	ID() string
}

// FormatNameWithID conditionally formats `template`. `template` is expected to have a single %s verb.
// If `id` is empty, the %s verb will be formatted with empty string. Otherwise %s verb will be replaced with `-id`.
// Eg:
// FormatNameWithID("name%s", "") returns "name"
// FormatNameWithID("name%s", "ns1-es1") returns "name-ns1-es2"
// FormatNameWithID("name%s", "ns2-es2") returns "name-ns2-es2"
// This function exists to abstract this conditional logic away from the callers. It can be used to format names
// for objects differing only by id, that would otherwise collide. In addition, it allows to preserve current naming
// for object types with a single instance and introduce object types with unbounded number of ids.
func FormatNameWithID(template string, id string) string {
	if id != "" {
		// we want names to be changed for any but empty id
		id = fmt.Sprintf("-%s", id)
	}

	return fmt.Sprintf(template, id)
}

// AssociationConf holds the association configuration of a referenced resource in an association.
type AssociationConf struct {
	AuthSecretName string `json:"authSecretName"`
	AuthSecretKey  string `json:"authSecretKey"`
	CACertProvided bool   `json:"caCertProvided"`
	CASecretName   string `json:"caSecretName"`
	URL            string `json:"url"`
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

// AuthIsConfigured returns true if all the auth fields are set.
func (ac *AssociationConf) AuthIsConfigured() bool {
	if ac == nil {
		return false
	}
	return ac.AuthSecretName != "" && ac.AuthSecretKey != ""
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
