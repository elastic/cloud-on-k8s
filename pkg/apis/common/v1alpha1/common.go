// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ReconcilerStatus represents status information about desired/available nodes.
type ReconcilerStatus struct {
	AvailableNodes int `json:"availableNodes,omitempty"`
}

// SecretRef reference a secret by name.
type SecretRef struct {
	SecretName string `json:"secretName,omitempty"`
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

// IsDefined checks if the object selector is not nil and has a name.
// Namespace is not mandatory as it may be inherited by the parent object.
func (s *ObjectSelector) IsDefined() bool {
	return s != nil && s.Name != ""
}

// HTTPConfig configures an HTTP-based service.
type HTTPConfig struct {
	// Service is a template for the Kubernetes Service
	Service ServiceTemplate `json:"service,omitempty"`
	// TLS describe additional options to consider when generating HTTP TLS certificates.
	TLS TLSOptions `json:"tls,omitempty"`
}

// Scheme returns the scheme for this HTTP config
func (http HTTPConfig) Scheme() string {
	if http.TLS.Enabled() {
		return "https"
	}
	return "http"
}

type TLSOptions struct {
	// SelfSignedCertificate define options to apply to self-signed certificate
	// managed by the operator.
	SelfSignedCertificate *SelfSignedCertificate `json:"selfSignedCertificate,omitempty"`

	// Certificate is a reference to a secret that contains the certificate and private key to be used.
	//
	// The secret should have the following content:
	//
	// - `ca.crt`: The certificate authority (optional)
	// - `tls.crt`: The certificate (or a chain).
	// - `tls.key`: The private key to the first certificate in the certificate chain.
	Certificate SecretRef `json:"certificate,omitempty"`
}

// Enabled returns true when TLS is enabled based on this option struct.
func (tls TLSOptions) Enabled() bool {
	selfSigned := tls.SelfSignedCertificate
	return selfSigned == nil || !selfSigned.Disabled || tls.Certificate.SecretName != ""
}

type SelfSignedCertificate struct {
	// SubjectAlternativeNames is a list of SANs to include in the HTTP TLS certificates.
	// For example: a wildcard DNS to expose the cluster.
	SubjectAlternativeNames []SubjectAlternativeName `json:"subjectAltNames,omitempty"`
	// Disabled turns off the provisioning of self-signed HTTP TLS certificates.
	Disabled bool `json:"disabled,omitempty"`
}

type SubjectAlternativeName struct {
	DNS string `json:"dns,omitempty"`
	IP  string `json:"ip,omitempty"`
}

// ServiceTemplate describes the data a service should have when created from a template
type ServiceTemplate struct {
	// ObjectMeta is metadata for the service.
	// The name and namespace provided here is managed by ECK and will be ignored.
	// +optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the behavior of the service.
	// +optional
	Spec v1.ServiceSpec `json:"spec,omitempty"`
}

// DefaultPodDisruptionBudgetMaxUnavailable is the default max unavailable pods in a PDB.
var DefaultPodDisruptionBudgetMaxUnavailable = intstr.FromInt(1)

// PodDisruptionBudgetTemplate contains a template for creating a PodDisruptionBudget.
type PodDisruptionBudgetTemplate struct {
	// ObjectMeta is metadata for the service.
	// The name and namespace provided here is managed by ECK and will be ignored.
	// +optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec of the desired behavior of the PodDisruptionBudget
	// +optional
	Spec v1beta1.PodDisruptionBudgetSpec `json:"spec,omitempty"`
}

type SecretSource struct {
	// Name of the secret in the pod's namespace to use.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#secret
	SecretName string `json:"secretName"`
	// If unspecified, each key-value pair in the Data field of the referenced
	// Secret will be projected into the volume as a file whose name is the
	// key and content is the value. If specified, the listed keys will be
	// projected into the specified paths, and unlisted keys will not be
	// present.
	// +optional
	Entries []KeyToPath `json:"entries,omitempty"`
}

// Maps a string key to a path within a volume.
type KeyToPath struct {
	// The key to project.
	Key string `json:"key"`

	// The relative path of the file to map the key to.
	// May not be an absolute path.
	// May not contain the path element '..'.
	// May not start with the string '..'.
	// +optional
	Path string `json:"path,omitempty"`
}
