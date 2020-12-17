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

// AssociationType is the type of an association resource.
type AssociationType string

// AssociationStatus is the status of an association resource.
type AssociationStatus string

// AssociationStatusMap is the map of association to its AssociationStatus
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
		return "", fmt.Errorf("expected at most one key-value but found %d", len(asm))
	}

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

	// ID allows to distinguish between many associations of the same type
	ID() int
}

// FormatNameWithID conditionally formats `template`. `template` is expected to have
// a single %s verb. If `id` is 0, the %s verb will be formatted with empty string. Otherwise %s verb will be
// replaced with `-ordinal` where ordinal is `id`+1. Eg.:
// FormatNameWithID("name%s", 0) returns "name"
// FormatNameWithID("name%s", 1) returns "name-2"
// FormatNameWithID("name%s", 2) returns "name-3"
// This function can be used to format names for objects differing only by id, that would otherwise collide. It allows
// to preserve current naming for object types with a single id and introduce object types with unbounded number of ids.
func FormatNameWithID(template string, id int) string {
	idString := ""
	if id > 0 {
		// we want names to be changed only all but first id. When appending the id, we want it to start from 2, so user
		// sees "name", "name-2", "name-3", etc.
		idString = fmt.Sprintf("-%d", id+1)
	}

	return fmt.Sprintf(template, idString)
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
