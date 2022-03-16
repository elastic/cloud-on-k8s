// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeploymentHealth string

const (
	GreenHealth DeploymentHealth = "green"
	RedHealth   DeploymentHealth = "red"
)

// DeploymentStatus represents status information about a deployment.
type DeploymentStatus struct {
	// Selector is the label selector used to find all pods.
	Selector string `json:"selector,omitempty"`
	// Count corresponds to Scale.Status.Replicas, which is the actual number of observed instances of the scaled object.
	// +optional
	Count int32 `json:"count"`
	// AvailableNodes is the number of available replicas in the deployment.
	AvailableNodes int32 `json:"availableNodes,omitempty"`
	// Version of the stack resource currently running. During version upgrades, multiple versions may run
	// in parallel: this value specifies the lowest version currently running.
	Version string `json:"version,omitempty"`
	// Health of the deployment.
	Health DeploymentHealth `json:"health,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (ds DeploymentStatus) IsDegraded(prev DeploymentStatus) bool {
	return prev.Health == GreenHealth && ds.Health != GreenHealth
}

// SecretRef is a reference to a secret that exists in the same namespace.
type SecretRef struct {
	// SecretName is the name of the secret.
	SecretName string `json:"secretName,omitempty"`
}

// ObjectSelector defines a reference to a Kubernetes object which can be an Elastic custom resource or a Secret.
type ObjectSelector struct {
	// Namespace of the Kubernetes object. If empty, defaults to the current namespace.
	Namespace string `json:"namespace,omitempty"`

	// Name of an existing Kubernetes object corresponding to an Elastic custom resource managed by ECK.
	Name string `json:"name,omitempty"`

	// ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced
	// object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of
	// the referenced resource is used.
	ServiceName string `json:"serviceName,omitempty"`

	// SecretName is the name of an existing Kubernetes secret that contains connection information for associating an
	// Elastic resource not managed by ECK. The referenced secret must contain the following:
	// - `url`: the URL to reach the Elastic resource
	// - `username`: the username of the user to be authenticated to the Elastic resource
	// - `password`: the password of the user to be authenticated to the Elastic resource
	// - `ca.crt`: the CA certificate in PEM format (optional).
	// This field cannot be defined if a name or a serviceName is defined.
	SecretName string `json:"secretName,omitempty"`
}

// WithDefaultNamespace adds a default namespace to a given ObjectSelector if none is set.
func (o ObjectSelector) WithDefaultNamespace(defaultNamespace string) ObjectSelector {
	if len(o.Namespace) > 0 {
		return o
	}
	return ObjectSelector{
		Namespace:   defaultNamespace,
		Name:        o.Name,
		ServiceName: o.ServiceName,
		SecretName:  o.SecretName,
	}
}

func (o ObjectSelector) NameOrSecretName() string {
	name := o.Name
	if o.SecretName != "" {
		name = o.SecretName
	}
	return name
}

// NamespacedName is a convenience method to turn an ObjectSelector into a NamespacedName.
func (o ObjectSelector) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      o.NameOrSecretName(),
		Namespace: o.Namespace,
	}
}

// IsDefined checks if the object selector is not nil and has a name or a secret name.
// Namespace is not mandatory as it may be inherited by the parent object.
func (o *ObjectSelector) IsDefined() bool {
	return o != nil && o.NameOrSecretName() != ""
}

// IsObjectTypeSecret returns true when the object selector references a Kubernetes secret describing the referenced object.
func (o ObjectSelector) IsObjectTypeSecret() bool {
	return o.IsDefined() && o.SecretName != ""
}

func (o ObjectSelector) String() string {
	return fmt.Sprintf("%s-%s", o.Namespace, o.NameOrSecretName())
}

// HTTPConfig holds the HTTP layer configuration for resources.
type HTTPConfig struct {
	// Service defines the template for the associated Kubernetes Service object.
	Service ServiceTemplate `json:"service,omitempty"`
	// TLS defines options for configuring TLS for HTTP.
	TLS TLSOptions `json:"tls,omitempty"`
}

// Protocol returns the inferrred protocol (http or https) for this configuration.
func (http HTTPConfig) Protocol() string {
	if http.TLS.Enabled() {
		return "https"
	}
	return "http"
}

// TLSOptions holds TLS configuration options.
type TLSOptions struct {
	// SelfSignedCertificate allows configuring the self-signed certificate generated by the operator.
	SelfSignedCertificate *SelfSignedCertificate `json:"selfSignedCertificate,omitempty"`

	// Certificate is a reference to a Kubernetes secret that contains the certificate and private key for enabling TLS.
	// The referenced secret should contain the following:
	//
	// - `ca.crt`: The certificate authority (optional).
	// - `tls.crt`: The certificate (or a chain).
	// - `tls.key`: The private key to the first certificate in the certificate chain.
	Certificate SecretRef `json:"certificate,omitempty"`
}

// Enabled returns true when TLS is enabled based on this option struct.
func (tls TLSOptions) Enabled() bool {
	selfSigned := tls.SelfSignedCertificate
	return selfSigned == nil || !selfSigned.Disabled || tls.Certificate.SecretName != ""
}

// SelfSignedCertificate holds configuration for the self-signed certificate generated by the operator.
type SelfSignedCertificate struct {
	// SubjectAlternativeNames is a list of SANs to include in the generated HTTP TLS certificate.
	SubjectAlternativeNames []SubjectAlternativeName `json:"subjectAltNames,omitempty"`
	// Disabled indicates that the provisioning of the self-signed certifcate should be disabled.
	Disabled bool `json:"disabled,omitempty"`
}

// SubjectAlternativeName represents a SAN entry in a x509 certificate.
type SubjectAlternativeName struct {
	// DNS is the DNS name of the subject.
	DNS string `json:"dns,omitempty"`
	// IP is the IP address of the subject.
	IP string `json:"ip,omitempty"`
}

// ServiceTemplate defines the template for a Kubernetes Service.
type ServiceTemplate struct {
	// ObjectMeta is the metadata of the service.
	// The name and namespace provided here are managed by ECK and will be ignored.
	// +kubebuilder:validation:Optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the service.
	// +kubebuilder:validation:Optional
	Spec v1.ServiceSpec `json:"spec,omitempty"`
}

// DefaultPodDisruptionBudgetMaxUnavailable is the default max unavailable pods in a PDB.
var DefaultPodDisruptionBudgetMaxUnavailable = intstr.FromInt(1)

// PodDisruptionBudgetTemplate defines the template for creating a PodDisruptionBudget.
type PodDisruptionBudgetTemplate struct {
	// ObjectMeta is the metadata of the PDB.
	// The name and namespace provided here are managed by ECK and will be ignored.
	// +kubebuilder:validation:Optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the PDB.
	// +kubebuilder:validation:Optional
	Spec v1beta1.PodDisruptionBudgetSpec `json:"spec,omitempty"`
}

// IsDisabled returns true if the PodDisruptionBudget is explicitly disabled (not nil, but empty).
func (p *PodDisruptionBudgetTemplate) IsDisabled() bool {
	return reflect.DeepEqual(p, &PodDisruptionBudgetTemplate{})
}

// SecretSource defines a data source based on a Kubernetes Secret.
type SecretSource struct {
	// SecretName is the name of the secret.
	SecretName string `json:"secretName"`
	// Entries define how to project each key-value pair in the secret to filesystem paths.
	// If not defined, all keys will be projected to similarly named paths in the filesystem.
	// If defined, only the specified keys will be projected to the corresponding paths.
	// +kubebuilder:validation:Optional
	Entries []KeyToPath `json:"entries,omitempty"`
}

// KeyToPath defines how to map a key in a Secret object to a filesystem path.
type KeyToPath struct {
	// Key is the key contained in the secret.
	Key string `json:"key"`

	// Path is the relative file path to map the key to.
	// Path must not be an absolute file path and must not contain any ".." components.
	// +kubebuilder:validation:Optional
	Path string `json:"path,omitempty"`
}

// ConfigSource references configuration settings.
type ConfigSource struct {
	// SecretName references a Kubernetes Secret in the same namespace as the resource that will consume it.
	//
	// Examples:
	// ---
	// # Filebeat configuration
	// kind: Secret
	// apiVersion: v1
	// metadata:
	// 	 name: filebeat-user-config
	// stringData:
	//   beat.yml: |-
	//     filebeat.inputs:
	//     - type: container
	//       paths:
	//       - /var/log/containers/*.log
	//       processors:
	//       - add_kubernetes_metadata:
	//           node: ${NODE_NAME}
	//           matchers:
	//           - logs_path:
	//               logs_path: "/var/log/containers/"
	//     processors:
	//     - add_cloud_metadata: {}
	//     - add_host_metadata: {}
	// ---
	// # EnterpriseSearch configuration
	// kind: Secret
	// apiVersion: v1
	// metadata:
	// 	name: smtp-credentials
	// stringData:
	//  enterprise-search.yml: |-
	//    email.account.enabled: true
	//    email.account.smtp.auth: plain
	//    email.account.smtp.starttls.enable: false
	//    email.account.smtp.host: 127.0.0.1
	//    email.account.smtp.port: 25
	//    email.account.smtp.user: myuser
	//    email.account.smtp.password: mypassword
	//    email.account.email_defaults.from: my@email.com
	// ---
	SecretRef `json:",inline"`
}

// HasObservedGeneration allows a return of any object's observed generation.
// +kubebuilder:object:generate=false
type HasObservedGeneration interface {
	client.Object
	GetObservedGeneration() int64
}
