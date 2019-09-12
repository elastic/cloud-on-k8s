// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

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
	ElasticsearchRef() ObjectSelector
	AssociationConf() *AssociationConf
}

// Associator describes an object that allows its association to be set.
// +kubebuilder:object:generate=false
type Associator interface {
	metav1.Object
	runtime.Object
	SetAssociationConf(*AssociationConf)
}

// AssociationConf holds the association configuration of an Elasticsearch cluster.
type AssociationConf struct {
	AuthSecretName string `json:"authSecretName"`
	AuthSecretKey  string `json:"authSecretKey"`
	CASecretName   string `json:"caSecretName"`
	URL            string `json:"url"`
}

// IsConfigured returns true if all the fields are set.
func (esac *AssociationConf) IsConfigured() bool {
	return esac.AuthIsConfigured() && esac.CAIsConfigured() && esac.URLIsConfigured()
}

// AuthIsConfigured returns true if all the auth fields are set.
func (esac *AssociationConf) AuthIsConfigured() bool {
	if esac == nil {
		return false
	}
	return esac.AuthSecretName != "" && esac.AuthSecretKey != ""
}

// CAIsConfigured returns true if the CA field is set.
func (esac *AssociationConf) CAIsConfigured() bool {
	if esac == nil {
		return false
	}
	return esac.CASecretName != ""
}

// URLIsConfigured returns true if the URL field is set.
func (esac *AssociationConf) URLIsConfigured() bool {
	if esac == nil {
		return false
	}
	return esac.URL != ""
}

func (esac *AssociationConf) GetAuthSecretName() string {
	if esac == nil {
		return ""
	}
	return esac.AuthSecretName
}

func (esac *AssociationConf) GetAuthSecretKey() string {
	if esac == nil {
		return ""
	}
	return esac.AuthSecretKey
}

func (esac *AssociationConf) GetCASecretName() string {
	if esac == nil {
		return ""
	}
	return esac.CASecretName
}

func (esac *AssociationConf) GetURL() string {
	if esac == nil {
		return ""
	}
	return esac.URL
}
