package v1alpha1

import (
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StackSpec defines the desired state of Stack
type StackSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Version represents the version of the stack
	Version string `json:"version,omitempty"`

	//TODO the new deployments API in EC(E) supports sequences of
	//Kibanas and Elasticsearch clusters per stack deployment

	// Elasticsearch specific configuration for the stack.
	Elasticsearch ElasticsearchSpec `json:"elasticsearch,omitempty"`

	//Kibana spec for this stack
	Kibana KibanaSpec `json:"kibana,omitempty"`
}

// ElasticsearchSpec defines the desired state of an Elasticsearch deployment.
type ElasticsearchSpec struct {
	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Elasticsearch Cluster must have.
	NodeCount int32 `json:"nodeCount,omitempty"`

	// SetVmMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	SetVmMaxMapCount bool `json:"setVmMaxMapCount,omitempty"`
}

type KibanaSpec struct {
	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Kibana deployment must have.
	NodeCount int32 `json:"nodeCount,omitempty"`
}

// StackStatus defines the observed state of Stack
type StackStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Elasticsearch status for the Stack.
	Elasticsearch ElasticsearchStatus `json:"elasticsearch,omitempty"`
}

// ElasticsearchStatus defines the observed state of an Elasticsearch cluster.
type ElasticsearchStatus struct {
	// Nodes represents the number of running pods that the controller
	// has created
	Nodes int32 `json:"nodes,omitempty"`

	// Additions represents the number of instances that have been added in the
	// lifetime of the "Elasticsearch Stack".
	Additions int32 `json:"additions,omitempty"`

	// Deletions represents the number of instances that have been deleted in
	// the lifetime of the  "Elasticsearch Stack".
	Deletions int32 `json:"deletions,omitempty"`
}

// NodeAdded updates the node count by 1
func (es *ElasticsearchStatus) NodeAdded() {
	defer es.added()
	atomic.AddInt32(&es.Nodes, 1)
}

// added increments the Elasticsearch additions value.
func (es *ElasticsearchStatus) added() {
	atomic.AddInt32(&es.Additions, 1)
}

// deleted increments the Elasticsearch deletions version.
func (es *ElasticsearchStatus) deleted() {
	atomic.AddInt32(&es.Deletions, 1)
}

// NodeDeleted updates the node count by 1
func (es *ElasticsearchStatus) NodeDeleted() {
	defer es.deleted()
	atomic.AddInt32(&es.Nodes, -1)
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Stack is the Schema for the stacks API
// +k8s:openapi-gen=true
type Stack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StackSpec   `json:"spec,omitempty"`
	Status StackStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StackList contains a list of Stack
type StackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Stack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Stack{}, &StackList{})
}
