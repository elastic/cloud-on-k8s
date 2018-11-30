package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ElasticsearchSpec defines the desired state of Elasticsearch
type ElasticsearchSpec struct {
	// Version represents the version of the stack
	Version string `json:"version,omitempty"`

	// Image represents the docker image that will be used.
	Image string `json:"image,omitempty"`

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	SetVMMaxMapCount bool `json:"setVmMaxMapCount,omitempty"`

	// Expose determines which service type to use for this workload. The
	// options are: `ClusterIP|LoadBalancer|NodePort`. Defaults to ClusterIP.
	// +kubebuilder:validation:Enum=ClusterIP,LoadBalancer,NodePort
	Expose string `json:"expose,omitempty"`

	// Topologies represent a list of node topologies to be part of the cluster
	Topologies []ElasticsearchTopologySpec `json:"topologies,omitempty"`

	// SnapshotRepository defines a snapshot repository to be used for automatic snapshots.
	SnapshotRepository SnapshotRepository `json:"snapshotRepository,omitempty"`

	// FeatureFlags are instance-specific flags that enable or disable specific experimental features
	FeatureFlags commonv1alpha1.FeatureFlags `json:"featureFlags,omitempty"`
}

// SnapshotRepositoryType as in gcs, AWS s3, file etc.
type SnapshotRepositoryType string

// Supported repository types
const (
	SnapshotRepositoryTypeGCS SnapshotRepositoryType = "gcs"
)

// SnapshotRepositorySettings specify a storage location for snapshots.
type SnapshotRepositorySettings struct {
	// BucketName is the name of the provider specific storage bucket to use.
	BucketName string `json:"bucketName,omitempty"`
	// Credentials is a reference to a secret containing credentials for the storage provider.
	Credentials v1.SecretReference `json:"credentials,omitempty"`
}

// SnapshotRepository specifies that the user wants automatic snapshots to happen and indicates where they should be stored.
type SnapshotRepository struct {
	// Type of repository
	// +kubebuilder:validation:Enum=gcs
	Type SnapshotRepositoryType `json:"type"`
	// Settings are provider specific repository settings
	Settings SnapshotRepositorySettings `json:"settings"`
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
	Resources commonv1alpha1.ResourcesSpec `json:"resources,omitempty"`

	// NodeCount defines how many nodes have this topology
	NodeCount int32 `json:"nodeCount,omitempty"`

	// PodTemplate is the object that describes the Elasticsearch pods.
	// +optional
	PodTemplate ElasticsearchPodTemplateSpec `json:"template,omitempty"`

	// VolumeClaimTemplates is a list of claims that pods are allowed to reference.
	// Every claim in this list must have at least one matching (by name) volumeMount in one
	// container in the template. A claim in this list takes precedence over
	// any volumes in the template, with the same name.
	// TODO: Define the behavior if a claim already exists with the same name.
	// TODO: define special behavior based on claim metadata.name. (e.g data / logs volumes)
	// +optional
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// ElasticsearchPodTemplateSpec describes the data a pod should have when created from a template
type ElasticsearchPodTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the pod.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec ElasticsearchPodSpec `json:"spec,omitempty"`
}

type ElasticsearchPodSpec struct {
	// Affinity is the pod's scheduling constraints
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
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

// ElasticsearchHealth is the health of the cluster as returned by the health API.
type ElasticsearchHealth string

// Possible traffic light states Elasticsearch health can have.
const (
	ElasticsearchRedHealth    ElasticsearchHealth = "red"
	ElasticsearchYellowHealth ElasticsearchHealth = "yellow"
	ElasticsearchGreenHealth  ElasticsearchHealth = "green"
)

// Less for ElasticsearchHealth means green > yellow > red
func (h ElasticsearchHealth) Less(other ElasticsearchHealth) bool {
	switch {
	case h == other:
		return false
	case h == ElasticsearchGreenHealth:
		return false
	case h == ElasticsearchYellowHealth && other == ElasticsearchRedHealth:
		return false
	default:
		return true
	}
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

// ElasticsearchStatus defines the observed state of Elasticsearch
type ElasticsearchStatus struct {
	commonv1alpha1.ReconcilerStatus
	Health      ElasticsearchHealth             `json:"health,omitempty"`
	Phase       ElasticsearchOrchestrationPhase `json:"phase,omitempty"`
	ClusterUUID string                          `json:"clusterUUID,omitempty"`
	MasterNode  string                          `json:"masterNode,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (es ElasticsearchStatus) IsDegraded(prev ElasticsearchStatus) bool {
	return es.Health.Less(prev.Health)
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ElasticsearchCluster is the Schema for the elasticsearches API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type ElasticsearchCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticsearchSpec   `json:"spec,omitempty"`
	Status ElasticsearchStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ElasticsearchClusterList contains a list of Elasticsearch
type ElasticsearchClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElasticsearchCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElasticsearchCluster{}, &ElasticsearchClusterList{})
}
