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
	AssociationUnknown     AssociationStatus = ""
	AssociationPending     AssociationStatus = "Pending"
	AssociationEstablished AssociationStatus = "Established"
	AssociationFailed      AssociationStatus = "Failed"
)

// Associated interface represents a Elastic stack application that is associated with an Elasticsearch cluster.
// An associated object needs some credentials to establish a connection to the Elasticsearch cluster and usually it
// offers a keystore which in ECK is represented with an underlying Secret.
// Kibana and the APM server are two examples of associated objects.
// +kubebuilder:object:generate=false
type Associated interface {
	metav1.Object
	runtime.Object
	ServiceAccountName() string
	GetAssociations() []Association
}

// Association interface helps to manage the Spec fields involved in a association.
// +kubebuilder:object:generate=false
type Association interface {
	Associated

	// Associated can be used to retrieve the associated object
	Associated() Associated

	// AssociatedServiceType returns a string describing the type of the target service (elasticsearch most of the time)
	// It is mostly used to build some other strings depending on the type of the targeted service.
	AssociatedServiceType() string

	// AnnotationName is the name of the annotation used to define the config for the associated resource.
	// It is used by the association controller to store the configuration and by the controller which is
	// managing the associated resource to build the appropriate configuration.
	AnnotationName() string

	// Remote service reference
	AssociationRef() ObjectSelector

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
	return ac.AuthIsConfigured() && ac.CAIsConfigured() && ac.URLIsConfigured()
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
