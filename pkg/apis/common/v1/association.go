// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AssociationType is the type of an association resource.
type AssociationType string

// AssociationStatus is the status of an association resource.
type AssociationStatus string

// AssociationStatusMap is the map of association to its AssociationStatus
type AssociationStatusMap map[string]AssociationStatus

func NewAssociationStatusGroup(nsName string, status AssociationStatus) AssociationStatusMap {
	return map[string]AssociationStatus{
		nsName: status,
	}
}

func (asg AssociationStatusMap) Single() (AssociationStatus, error) {
	if len(asg) > 1 {
		return "", fmt.Errorf("expected at most one key-value but found %d", len(asg))
	}

	var result AssociationStatus
	for _, status := range asg {
		result = status
	}
	return result, nil
}

func (asg AssociationStatusMap) Aggregate() AssociationStatus {
	worst := AssociationUnknown
	for _, status := range asg {
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
	AssociationStatusGroup(typ AssociationType) AssociationStatusMap
	SetAssociationStatusGroup(typ AssociationType, statusGroup AssociationStatusMap) error
}

// Association interface helps to manage the Spec fields involved in an association.
// +kubebuilder:object:generate=false
type Association interface {
	Associated

	// Associated can be used to retrieve the associated object
	Associated() Associated

	// AssociatedType returns a string describing the type of the target resource (elasticsearch most of the time)
	// It is mostly used to build some other strings depending on the type of the targeted resource.
	AssociatedType() AssociationType

	// Reference to the associated resource. If defined with a Name then the Namespace is expected to be set in the returned object.
	AssociationRef() ObjectSelector

	// AssociationConfAnnotationNameBase is the name of the annotation used to define the config for the associated resource.
	// It is used by the association controller to store the configuration and by the controller which is
	// managing the associated resource to build the appropriate configuration.
	AssociationConfAnnotationNameBase() string

	// Configuration
	AssociationConf() *AssociationConf
	SetAssociationConf(*AssociationConf)

	// Id allows to distinguish between many associations of the same type
	ID() int
}

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
