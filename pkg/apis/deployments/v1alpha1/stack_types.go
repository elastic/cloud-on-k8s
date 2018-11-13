package v1alpha1

import (
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

	// FeatureFlags are stack-specific flags that enable or disable specific experimental features
	FeatureFlags FeatureFlags `json:"featureFlags,omitempty"`

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

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	SetVMMaxMapCount bool `json:"setVmMaxMapCount,omitempty"`

	// Topologies represent a list of node topologies to be part of the cluster
	Topologies []ElasticsearchTopologySpec `json:"topologies,omitempty"`
}

// NodeCount returns the total number of nodes of the Elasticsearch cluster
func (es ElasticsearchSpec) NodeCount() int32 {
	count := int32(0)
	for _, t := range es.Topologies {
		count += t.NodeCount
	}
	return count
}

// ElasticsearchTopologySpec defines a common topology for a set of Elasticsearch nodes
type ElasticsearchTopologySpec struct {
	// NodeTypes represents the node type
	NodeTypes NodeTypesSpec `json:"nodeTypes,omitempty"`

	// Resources to be allocated for this topology
	Resources ResourcesSpec `json:"resources,omitempty"`

	// NodeCount defines how many nodes have this topology
	NodeCount int32 `json:"nodeCount,omitempty"`
}

// NodeTypesSpec define the
type NodeTypesSpec struct {
	// Master represents a master node
	Master bool `json:"master,omitempty"`
	// Data represents a data node
	Data bool `json:"data,omitempty"`
	// Ingest represents an ingest node
	Ingest bool `json:"ingest,omitempty"`
	// ML represents a machine learning node
	ML bool `json:"ml,omitempty"`
}

// KibanaSpec defines a Kibana deployment spec
type KibanaSpec struct {
	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// NodeCount defines how many nodes the Kibana deployment must have.
	NodeCount int32 `json:"nodeCount,omitempty"`

	// Resources to be allocated for this topology
	Resources ResourcesSpec `json:"resources,omitempty"`
}

// ResourcesSpec defines the resources to be allocated to a pod
type ResourcesSpec struct {
	// Limits represents the limits to considerate for these resources
	Limits LimitsSpec `json:"limits,omitempty"`
}

// LimitsSpec define limit in resources allocated to a pod
type LimitsSpec struct {
	// Memory is the maximum amount of memory to allocate
	Memory string `json:"memory,omitempty"`
	// Storage is the maximum amount of storage to allocate
	Storage string `json:"storage,omitempty"`
	// CPU is the maximum amount of CPU to allocate
	CPU string `json:"cpu,omitempty"`
}

// ElasticsearchHealth is the health of the cluster as returned by the health API.
type ElasticsearchHealth string

//Possible traffic light states Elasticsearch health can have.
const (
	ElasticsearchRedHealth    ElasticsearchHealth = "red"
	ElasticsearchYellowHealth ElasticsearchHealth = "yellow"
	ElasticsearchGreenHealth  ElasticsearchHealth = "green"
)

// ReconcilerStatus represents status information about desired/available nodes.
type ReconcilerStatus struct {
	AvailableNodes int
}

// ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.
type ElasticsearchOrchestrationPhase string

const (
	// ElasticsearchOperationalPhase is operating at the desired spec.
	ElasticsearchOperationalPhase ElasticsearchOrchestrationPhase = "Operational"
	// ElasticsearchPendingPhase controller is working towards a desired state, cluster can be unavailable.
	ElasticsearchPendingPhase ElasticsearchOrchestrationPhase = "Pending"
	// ElasticsearchMigratingDataPhase Elasticsearch is currently migrating data to another node.
	ElasticsearchMigratingDataPhase ElasticsearchOrchestrationPhase = "MigratingData"
)

// ElasticsearchStatus contains status information about the Elasticsearch cluster.
type ElasticsearchStatus struct {
	ReconcilerStatus
	Health ElasticsearchHealth
	Phase  ElasticsearchOrchestrationPhase
}

// KibanaHealth expresses the status of the Kibana instances.
type KibanaHealth string

const (
	// KibanaRed means no instance is currently available.
	KibanaRed KibanaHealth = "red"
	// KibanaGreen means at least one instance is available.
	KibanaGreen KibanaHealth = "green"
)

// KibanaStatus contains status information about the Kibana instances in the stack deployment.
type KibanaStatus struct {
	ReconcilerStatus
	Health KibanaHealth
}

// StackStatus defines the observed state of Stack
type StackStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Elasticsearch ElasticsearchStatus
	Kibana        KibanaStatus
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Stack is the Schema for the stacks API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
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
