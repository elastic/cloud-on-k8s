// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AssociationStatus is the status of an association resource.
type AssociationStatus string

const (
	ElasticsearchConfigAnnotationName = "association.k8s.elastic.co/es-conf"
	ElasticsearchAssociationType      = "elasticsearch"

	KibanaAssociationType      = "kibana"
	KibanaConfigAnnotationName = "association.k8s.elastic.co/kb-conf"

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
// +kubebuilder:object:generate=false
type Associated interface {
	metav1.Object
	runtime.Object
	ServiceAccountName() string
	GetAssociations() []Association
}

// Association interface helps to manage the Spec fields involved in an association.
// +kubebuilder:object:generate=false
type Association interface {
	Associated

	// Associated can be used to retrieve the associated object
	Associated() Associated

	// AssociatedType returns a string describing the type of the target resource (elasticsearch most of the time)
	// It is mostly used to build some other strings depending on the type of the targeted resource.
	AssociatedType() string

	// Reference to the associated resource. If defined with a Name then the Namespace is expected to be set in the returned object.
	AssociationRef() ObjectSelector

	// AssociationConfAnnotationName is the name of the annotation used to define the config for the associated resource.
	// It is used by the association controller to store the configuration and by the controller which is
	// managing the associated resource to build the appropriate configuration.
	AssociationConfAnnotationName() string

	// Configuration
	AssociationConf() *AssociationConf
	SetAssociationConf(*AssociationConf)

	// Status
	AssociationStatus() AssociationStatus
	SetAssociationStatus(status AssociationStatus)
}

// AssociationConf holds the association configuration of an Elasticsearch cluster.
type AssociationConf struct {
	AuthSecretName string `json:"authSecretName"`
	AuthSecretKey  string `json:"authSecretKey"`
	CACertProvided bool   `json:"caCertProvided"`
	CASecretName   string `json:"caSecretName"`
	URL            string `json:"url"`
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
