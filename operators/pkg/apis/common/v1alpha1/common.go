// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/types"
)

// ReconcilerStatus represents status information about desired/available nodes.
type ReconcilerStatus struct {
	AvailableNodes int `json:"availableNodes,omitempty"`
}

// SecretRef reference a secret by name.
type SecretRef struct {
	SecretName string `json:"secretName"`
}

// ObjectSelector allows to specify a reference to an object across namespace boundaries.
type ObjectSelector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// NamespacedName is a convenience method to turn an ObjectSelector into a NamespaceName.
func (s ObjectSelector) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      s.Name,
		Namespace: s.Namespace,
	}
}

// IsDefined checks if the object selector is not nil and has both a name and a namespace.
func (s *ObjectSelector) IsDefined() bool {
	return s != nil && s.Name != "" && s.Namespace != ""
}

// HTTPConfig configures a HTTP-based service.
type HTTPConfig struct {
	// Service is a template for the Kubernetes Service
	Service HTTPService `json:"service,omitempty"`
	// TLS describe additional options to consider when generating HTTP TLS certificates.
	TLS TLSOptions `json:"tls,omitempty"`
}

type TLSOptions struct {
	// SelfSignedCertificate define options to apply to self-signed certificate
	// managed by the operator.
	SelfSignedCertificate *SelfSignedCertificate `json:"selfSignedCertificate,omitempty"`
}

type SelfSignedCertificate struct {
	// SubjectAlternativeNames is a list of SANs to include in the HTTP TLS certificates.
	// For example: a wildcard DNS to expose the cluster.
	SubjectAlternativeNames []SubjectAlternativeName `json:"subjectAltNames,omitempty"`
}

type SubjectAlternativeName struct {
	DNS string `json:"dns,omitempty"`
	IP  string `json:"ip,omitempty"`
}

// HTTPService contains defaults for a HTTP service.
type HTTPService struct {
	// Metadata is metadata for the HTTP Service.
	Metadata HTTPServiceObjectMeta `json:"metadata,omitempty"`

	// Spec contains user-provided settings for the HTTP Service.
	Spec HTTPServiceSpec `json:"spec,omitempty"`
}

// HTTPServiceObjectMeta is metadata for HTTP Service.
type HTTPServiceObjectMeta struct {
	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// HTTPServiceSpec contains a subset of overridable settings for the HTTP Service
type HTTPServiceSpec struct {
	// Type determines which service type to use for this workload. The
	// options are: `ClusterIP|LoadBalancer|NodePort`. Defaults to ClusterIP.
	// +kubebuilder:validation:Enum=ClusterIP,LoadBalancer,NodePort
	Type string `json:"type,omitempty"`
}
