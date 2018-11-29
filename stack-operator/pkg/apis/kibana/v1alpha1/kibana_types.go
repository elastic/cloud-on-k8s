package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KibanaSpec defines the desired state of Kibana
type KibanaSpec struct {
	// Version represents the version of Kibana
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Kibana deployment must have.
	NodeCount int32 `json:"nodeCount,omitempty"`

	// Elasticsearch configures how Kibana connects to Elasticsearch
	Elasticsearch BackendElasticsearch `json:"elasticsearch"`

	// Expose determines which service type to use for this workload. The
	// options are: `ClusterIP|LoadBalancer|NodePort`. Defaults to ClusterIP.
	// +kubebuilder:validation:Enum=ClusterIP,LoadBalancer,NodePort
	Expose string `json:"expose,omitempty"`

	// Resources to be allocated for this topology
	Resources commonv1alpha1.ResourcesSpec `json:"resources,omitempty"`

	// FeatureFlags are instance-specific flags that enable or disable specific experimental features
	FeatureFlags commonv1alpha1.FeatureFlags `json:"featureFlags,omitempty"`
}

// BackendElasticsearch contains configuration for an Elasticsearch backend for Kibana
type BackendElasticsearch struct {
	// ElasticsearchURL is the URL to the target Elasticsearch
	URL string `json:"url"`

	// Auth configures authentication for Kibana to use.
	Auth ElasticsearchAuth `json:"auth"`

	// CaCertSecret names a secret that contains a ca.pem file entry to use.
	CaCertSecret *string `json:"caCertSecret,omitempty"`
}

// ElasticsearchAuth contains auth config for Kibana to use with an Elasticsearch cluster
type ElasticsearchAuth struct {
	// Inline is auth provided as plaintext inline credentials.
	Inline *ElasticsearchInlineAuth `json:"inline,omitempty"`
}

// ElasticsearchInlineAuth is a basic username/password combination.
type ElasticsearchInlineAuth struct {
	// User is the username to use.
	Username string `json:"username"`
	// Password is the password to use.
	Password string `json:"password"`
}

// KibanaHealth expresses the status of the Kibana instances.
type KibanaHealth string

const (
	// KibanaRed means no instance is currently available.
	KibanaRed KibanaHealth = "red"
	// KibanaGreen means at least one instance is available.
	KibanaGreen KibanaHealth = "green"
)

// KibanaStatus defines the observed state of Kibana
type KibanaStatus struct {
	commonv1alpha1.ReconcilerStatus
	Health KibanaHealth
}

// IsDegraded returns true if the current status is worse than the previous.
func (ks KibanaStatus) IsDegraded(prev KibanaStatus) bool {
	return prev.Health == KibanaGreen && ks.Health != KibanaGreen
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Kibana is the Schema for the kibanas API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Kibana struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KibanaSpec   `json:"spec,omitempty"`
	Status KibanaStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KibanaList contains a list of Kibana
type KibanaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kibana `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kibana{}, &KibanaList{})
}
