// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"errors"
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
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
	// Conditions holds the current service state of the deployment.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (ds DeploymentStatus) IsDegraded(prev DeploymentStatus) bool {
	return prev.Health == GreenHealth && ds.Health != GreenHealth
}

// ConfigMapRef is a reference to a config map that exists in the same namespace as the referring resource.
type ConfigMapRef struct {
	ConfigMapName string `json:"configMapName,omitempty"`
}

func (c ConfigMapRef) IsDefined() bool {
	return len(c.ConfigMapName) > 0
}

// SecretRef is a reference to a secret that exists in the same namespace.
type SecretRef struct {
	// SecretName is the name of the secret.
	SecretName string `json:"secretName,omitempty"`
}

var _ AssociationRef = (*LocalObjectSelector)(nil)

// LocalObjectSelector defines a reference to a Kubernetes object corresponding to an Elastic resource managed by the operator
type LocalObjectSelector struct {
	// Namespace of the Kubernetes object. If empty, defaults to the current namespace.
	Namespace string `json:"namespace,omitempty"`

	// Name of an existing Kubernetes object corresponding to an Elastic resource managed by ECK.
	Name string `json:"name,omitempty"`

	// ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced
	// object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of
	// the referenced resource is used.
	ServiceName string `json:"serviceName,omitempty"`
}

// GetClientCertificateSecretName always returns empty string as LocalObjectSelector does not support client certificates.
func (o LocalObjectSelector) GetClientCertificateSecretName() string {
	return ""
}

// WithDefaultNamespace adds a default namespace to a given LocalObjectSelector if none is set.
func (o LocalObjectSelector) WithDefaultNamespace(defaultNamespace string) LocalObjectSelector {
	if len(o.Namespace) > 0 {
		return o
	}
	return LocalObjectSelector{
		Namespace:   defaultNamespace,
		Name:        o.Name,
		ServiceName: o.ServiceName,
	}
}

// NamespacedName is a convenience method to turn a LocalObjectSelector into a NamespacedName.
func (o LocalObjectSelector) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      o.Name,
		Namespace: o.Namespace,
	}
}

// IsDefined checks if the local object selector is not nil and has a name.
// Namespace is not mandatory as it may be inherited by the parent object.
//
// Deprecated: Use IsSet instead.
func (o *LocalObjectSelector) IsDefined() bool {
	if o == nil {
		return false
	}
	return o.IsSet()
}

// IsSet checks if the local object selector has a name.
// Namespace is not mandatory as it may be inherited by the parent object.
func (o LocalObjectSelector) IsSet() bool {
	return o.Name != ""
}

// NameOrSecretName returns the name. LocalObjectSelector does not support external references.
func (o LocalObjectSelector) NameOrSecretName() string {
	return o.Name
}

// IsExternal always returns false as LocalObjectSelector does not support external references.
func (o LocalObjectSelector) IsExternal() bool {
	return false
}

// GetName returns the name of the LocalObjectSelector.
func (o LocalObjectSelector) GetName() string {
	return o.Name
}

// GetNamespace returns the namespace of the LocalObjectSelector.
func (o LocalObjectSelector) GetNamespace() string {
	return o.Namespace
}

// GetSecretName always returns empty string as LocalObjectSelector does not support external references.
func (o LocalObjectSelector) GetSecretName() string {
	return ""
}

// GetServiceName returns the service name of the LocalObjectSelector.
func (o LocalObjectSelector) GetServiceName() string {
	return o.ServiceName
}

// IsValid validates the LocalObjectSelector.
func (o LocalObjectSelector) IsValid() error {
	if o.Name == "" && o.ServiceName != "" {
		return errors.New("serviceName can only be used in combination with name")
	}
	if o.Name == "" && o.Namespace != "" {
		return errors.New("namespace can only be used in combination with name")
	}
	return nil
}

var _ AssociationRef = (*ObjectSelector)(nil)

// ObjectSelector defines a reference to a Kubernetes object which can be an Elastic resource managed by the operator
// or a Secret describing an external Elastic resource not managed by the operator.
type ObjectSelector struct {
	// Namespace of the Kubernetes object. If empty, defaults to the current namespace.
	Namespace string `json:"namespace,omitempty"`

	// Name of an existing Kubernetes object corresponding to an Elastic resource managed by ECK.
	Name string `json:"name,omitempty"`

	// ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced
	// object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of
	// the referenced resource is used.
	ServiceName string `json:"serviceName,omitempty"`

	// SecretName is the name of an existing Kubernetes secret that contains connection information for associating an
	// Elastic resource not managed by the operator.
	// The referenced secret must contain the following:
	// - `url`: the URL to reach the Elastic resource
	// - `username`: the username of the user to be authenticated to the Elastic resource
	// - `password`: the password of the user to be authenticated to the Elastic resource
	// - `ca.crt`: the CA certificate in PEM format (optional)
	// - `api-key`: the key to authenticate against the Elastic resource instead of a username and password (supported only for `elasticsearchRefs` in AgentSpec and in BeatSpec)
	// This field cannot be used in combination with the other fields name, namespace or serviceName.
	SecretName string `json:"secretName,omitempty"`
}

// GetClientCertificateSecretName always returns empty string as ObjectSelector does not support client certificates.
func (o ObjectSelector) GetClientCertificateSecretName() string {
	return ""
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

// NameOrSecretName returns the name or the secret name of the ObjectSelector.
// Name or secret name are mutually exclusive. Validation rules ensure that exactly one of the two is set.
func (o ObjectSelector) NameOrSecretName() string {
	if o.SecretName != "" {
		return o.SecretName
	}
	return o.Name
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
//
// Deprecated: Use IsSet instead.
func (o *ObjectSelector) IsDefined() bool {
	if o == nil {
		return false
	}
	return o.IsSet()
}

// IsExternal returns true when the object selector references a Kubernetes secret describing an external
// referenced object not managed by the operator.
func (o ObjectSelector) IsExternal() bool {
	return o.IsSet() && o.SecretName != ""
}

// IsSet checks if the object selector has a name or a secret name set.
func (o ObjectSelector) IsSet() bool {
	return o.NameOrSecretName() != ""
}

// GetName returns the name of the ObjectSelector.
func (o ObjectSelector) GetName() string {
	return o.Name
}

// GetNamespace returns the namespace of the ObjectSelector.
func (o ObjectSelector) GetNamespace() string {
	return o.Namespace
}

// GetSecretName returns the secret name of the ObjectSelector.
func (o ObjectSelector) GetSecretName() string {
	return o.SecretName
}

// GetServiceName returns the service name of the ObjectSelector.
func (o ObjectSelector) GetServiceName() string {
	return o.ServiceName
}

func (o ObjectSelector) IsValid() error {
	if o.Name != "" && o.SecretName != "" {
		return errors.New("specify name or secretName, not both")
	}
	if o.SecretName != "" && (o.ServiceName != "" || o.Namespace != "") {
		return errors.New("serviceName or namespace can only be used in combination with name, not with secretName")
	}
	if o.Name == "" && (o.ServiceName != "") {
		return errors.New("serviceName can only be used in combination with name")
	}
	if o.Name == "" && (o.Namespace != "") {
		return errors.New("namespace can only be used in combination with name")
	}
	return nil
}

// ToID returns a string representing the object selector that can be used as a unique identifier.
func (o ObjectSelector) ToID() string {
	if o.Namespace != "" {
		return fmt.Sprintf("%s-%s", o.Namespace, o.NameOrSecretName())
	}
	return o.NameOrSecretName()
}

var _ AssociationRef = (*ElasticsearchSelector)(nil)

// ElasticsearchSelector defines a reference to an Elasticsearch cluster managed by the operator
// or a Secret describing an external cluster not managed by the operator.
type ElasticsearchSelector struct {
	ObjectSelector `json:",inline"`

	// ClientCertificateSecretName is the name of an existing Kubernetes secret containing a client certificate
	// (tls.crt) and private key (tls.key) for client authentication to the referenced resource.
	// This field is only relevant when the referenced Elasticsearch cluster has client authentication enabled.
	// If not specified and the referenced resource requires client authentication, ECK will auto-generate a
	// client certificate.
	ClientCertificateSecretName string `json:"clientCertificateSecretName,omitempty"`
}

// GetClientCertificateSecretName returns the name of the client certificate secret.
func (e ElasticsearchSelector) GetClientCertificateSecretName() string {
	return e.ClientCertificateSecretName
}

// IsValid validates the ElasticsearchSelector, including the embedded ObjectSelector.
func (e ElasticsearchSelector) IsValid() error {
	if err := e.ObjectSelector.IsValid(); err != nil {
		return err
	}
	if e.Name == "" && e.ClientCertificateSecretName != "" {
		return errors.New("clientCertificateSecretName can only be used in combination with name")
	}
	return nil
}

// WithDefaultNamespace adds a default namespace to a given ElasticsearchSelector if none is set.
func (e ElasticsearchSelector) WithDefaultNamespace(defaultNamespace string) ElasticsearchSelector {
	if len(e.Namespace) > 0 {
		return e
	}
	return ElasticsearchSelector{
		ObjectSelector:              e.ObjectSelector.WithDefaultNamespace(defaultNamespace),
		ClientCertificateSecretName: e.ClientCertificateSecretName,
	}
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

// HTTPConfigWithClientOptions holds the HTTP layer configuration for resources.
type HTTPConfigWithClientOptions struct {
	// Service defines the template for the associated Kubernetes Service object.
	Service ServiceTemplate `json:"service,omitempty"`
	// TLS defines options for configuring TLS for HTTP.
	TLS TLSWithClientOptions `json:"tls,omitempty"`
}

// Protocol returns the inferred protocol (http or https) for this configuration.
func (http HTTPConfigWithClientOptions) Protocol() string {
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

// TLSWithClientOptions extends TLSOptions with client authentication settings.
type TLSWithClientOptions struct {
	TLSOptions `json:",inline"`

	// Client holds client configuration options.
	Client ClientOptions `json:"client,omitempty"`
}

// Enabled returns true when TLS is enabled based on this option struct.
func (tls TLSWithClientOptions) Enabled() bool {
	return tls.TLSOptions.Enabled()
}

// ClientOptions configures client certificate authentication for incoming connections.
type ClientOptions struct {
	// Authentication enables client authentication (enterprise-only feature).
	Authentication bool `json:"authentication,omitempty"`
}

// SelfSignedCertificate holds configuration for the self-signed certificate generated by the operator.
type SelfSignedCertificate struct {
	// SubjectAlternativeNames is a list of SANs to include in the generated HTTP TLS certificate.
	SubjectAlternativeNames []SubjectAlternativeName `json:"subjectAltNames,omitempty"`
	// Disabled indicates that the provisioning of the self-signed certificate should be disabled.
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
	Spec policyv1.PodDisruptionBudgetSpec `json:"spec,omitempty"`
}

// IsDisabled returns true if the PodDisruptionBudget is explicitly disabled (not nil, but empty).
func (p *PodDisruptionBudgetTemplate) IsDisabled() bool {
	return reflect.DeepEqual(p, &PodDisruptionBudgetTemplate{})
}

// IsSpecified returns true if the PodDisruptionBudget is specified (not nil, and has a non-empty spec).
func (p *PodDisruptionBudgetTemplate) IsSpecified() bool {
	return p != nil && (p.Spec.Selector != nil ||
		p.Spec.MinAvailable != nil ||
		p.Spec.MaxUnavailable != nil ||
		p.Spec.UnhealthyPodEvictionPolicy != nil)
}

// NamespacedSecretSource defines a data source based on a Kubernetes Secret in a given namespace.
type NamespacedSecretSource struct {
	// Namespace is the namespace of the secret.
	Namespace string `json:"namespace"`
	// SecretName is the name of the secret.
	SecretName string `json:"secretName"`
	// Entries define how to project each key-value pair in the secret to filesystem paths.
	// If not defined, all keys will be projected to similarly named paths in the filesystem.
	// If defined, only the specified keys will be projected to the corresponding paths.
	// +kubebuilder:validation:Optional
	Entries []KeyToPath `json:"entries,omitempty"`
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
	//     - type: filestream
	//       paths:
	//       - /var/log/containers/*.log
	//       parsers:
	//         - container: ~
	//       prospector:
	//         scanner:
	//           fingerprint.enabled: true
	//           symlinks: true
	//       file_identity.fingerprint: ~
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

// TypeLabelName is used to represent a resource type in k8s resources
const TypeLabelName = "common.k8s.elastic.co/type"

// HasIdentityLabels allows a return of Elastic assigned labels for any object.
// +kubebuilder:object:generate=false
type HasIdentityLabels interface {
	client.Object
	GetIdentityLabels() map[string]string
}

// DisableDowngradeValidationAnnotation allows circumventing downgrade/upgrade checks.
const DisableDowngradeValidationAnnotation = "eck.k8s.elastic.co/disable-downgrade-validation"

// IsConfiguredToAllowDowngrades returns true if the DisableDowngradeValidation annotation is set to the value of true.
func IsConfiguredToAllowDowngrades(o metav1.Object) bool {
	val, exists := o.GetAnnotations()[DisableDowngradeValidationAnnotation]
	return exists && val == "true"
}
