package support

import (
	commonv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// HTTPPort used by Elasticsearch for the REST API
	HTTPPort = 9200
	// TransportPort used by Elasticsearch for the Transport protocol
	TransportPort = 9300
	// TransportClientPort used by Elasticsearch for the Transport protocol for client-only connections
	TransportClientPort = 9400

	// DefaultImageRepository is the default image name without a tag
	DefaultImageRepository string = "docker.elastic.co/elasticsearch/elasticsearch"

	// DefaultTerminationGracePeriodSeconds is the termination grace period for the Elasticsearch containers
	DefaultTerminationGracePeriodSeconds int64 = 120

	// DefaultContainerName is the name of the elasticsearch container
	DefaultContainerName = "elasticsearch"
)

var (
	// DefaultContainerPorts are the default Elasticsearch port mappings
	DefaultContainerPorts = []corev1.ContainerPort{
		{Name: "http", ContainerPort: HTTPPort, Protocol: corev1.ProtocolTCP},
		{Name: "transport", ContainerPort: TransportPort, Protocol: corev1.ProtocolTCP},
		{Name: "client", ContainerPort: TransportClientPort, Protocol: corev1.ProtocolTCP},
	}
)

// NewPodSpecParams is used to build resources associated with an Elasticsearch Cluster
type NewPodSpecParams struct {
	// Version is the Elasticsearch version
	Version string
	// CustomImageName is the custom image used, leave empty for the default
	CustomImageName string
	// ClusterName is the name of the Elasticsearch cluster
	ClusterName string
	// DiscoveryServiceName is the name of the Service that should be used for discovery.
	DiscoveryServiceName string
	// DiscoveryZenMinimumMasterNodes is the setting for minimum master node in Zen Discovery
	DiscoveryZenMinimumMasterNodes int
	// NodeTypes defines the type (master/data/ingest) associated to the ES node
	NodeTypes v1alpha1.NodeTypesSpec

	// Affinity is the pod's scheduling constraints
	Affinity *corev1.Affinity

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	SetVMMaxMapCount bool

	// Resources is the memory/cpu resources the pod wants
	Resources commonv1alpha1.ResourcesSpec

	// UsersSecretVolume is the volume that contains x-pack configuration (users, users_roles)
	UsersSecretVolume SecretVolume
	// ExtraFilesRef is a reference to a secret containing generic extra resources for the pod.
	ExtraFilesRef types.NamespacedName
	// KeystoreConfig is configuration for the Elasticsearch key store setup
	KeystoreConfig keystore.Config
	// ProbeUser is the user that should be used for the readiness probes.
	ProbeUser client.User
}

// PodSpecContext contains a PodSpec and some additional context pertaining to its creation.
type PodSpecContext struct {
	PodSpec      corev1.PodSpec
	TopologySpec v1alpha1.ElasticsearchTopologySpec
}
