package elasticsearch

import (
	"strconv"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/initcontainer"
	"github.com/mitchellh/hashstructure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// HTTPPort used by Elasticsearch for the REST API
	HTTPPort = 9200
	// TransportPort used by Elasticsearch for the Transport protocol
	TransportPort = 9300
	// TransportClientPort used by Elasticsearch for the Transport protocol for client-only connections
	TransportClientPort = 9400

	// defaultImageRepositoryAndName is the default image name without a tag
	defaultImageRepositoryAndName string = "docker.elastic.co/elasticsearch/elasticsearch"

	// defaultTerminationGracePeriodSeconds is the termination grace period for the Elasticsearch containers
	defaultTerminationGracePeriodSeconds int64 = 120

	// containerName is the name of the elasticsearch container
	containerName = "elasticsearch"
)

var (
	// defaultContainerPorts are the default Elasticsearch port mappings
	defaultContainerPorts = []corev1.ContainerPort{
		{Name: "http", ContainerPort: HTTPPort, Protocol: corev1.ProtocolTCP},
		{Name: "transport", ContainerPort: TransportPort, Protocol: corev1.ProtocolTCP},
		{Name: "client", ContainerPort: TransportClientPort, Protocol: corev1.ProtocolTCP},
	}

	log = logf.Log.WithName("pod")
)

// NewPod constructs a pod from the given parameters.
func NewPod(stack deploymentsv1alpha1.Stack, podSpec corev1.PodSpec) (corev1.Pod, error) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NewNodeName(stack.Name),
			Namespace: stack.Namespace,
			Labels:    NewLabels(stack, true),
		},
		Spec: podSpec,
	}

	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		log.Info("Node certificates feature flag enabled", "pod", pod.Name)
		pod = configureNodeCertificates(pod)
	}

	return pod, nil
}

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
	DiscoveryZenMinimumMasterNodes int `hash:"ignore"`
	// NodeTypes defines the type (master/data/ingest) associated to the ES node
	NodeTypes deploymentsv1alpha1.NodeTypesSpec

	// SetVMMaxMapCount indicates whether a init container should be used to ensure that the `vm.max_map_count`
	// is set according to https://www.elastic.co/guide/en/elasticsearch/reference/current/vm-max-map-count.html.
	// Setting this to true requires the kubelet to allow running privileged containers.
	SetVMMaxMapCount bool
}

// Hash computes a unique hash with the current NewPodSpecParams
func (params NewPodSpecParams) Hash() string {
	hash, _ := hashstructure.Hash(params, nil)
	return strconv.FormatUint(hash, 10)
}

// CreateExpectedPodSpecs creates PodSpec for all Elasticsearch nodes in the given stack
func CreateExpectedPodSpecs(s deploymentsv1alpha1.Stack, probeUser client.User, extraFilesRef types.NamespacedName) ([]corev1.PodSpec, error) {
	podSpecs := make([]corev1.PodSpec, 0, s.Spec.Elasticsearch.NodeCount())
	for _, topology := range s.Spec.Elasticsearch.Topologies {
		for i := int32(0); i < topology.NodeCount; i++ {
			podSpec, err := NewPodSpec(NewPodSpecParams{
				Version:                        s.Spec.Version,
				CustomImageName:                s.Spec.Elasticsearch.Image,
				ClusterName:                    s.Name,
				DiscoveryZenMinimumMasterNodes: ComputeMinimumMasterNodes(s.Spec.Elasticsearch.Topologies),
				DiscoveryServiceName:           DiscoveryServiceName(s.Name),
				NodeTypes:                      topology.NodeTypes,
				SetVMMaxMapCount:               s.Spec.Elasticsearch.SetVMMaxMapCount,
			}, probeUser, extraFilesRef)
			if err != nil {
				return nil, err
			}
			podSpecs = append(podSpecs, podSpec)
		}
	}
	return podSpecs, nil
}

// NewPodSpec creates a new PodSpec for an Elasticsearch instance in this cluster.
func NewPodSpec(p NewPodSpecParams, probeUser client.User, extraFilesRef types.NamespacedName) (corev1.PodSpec, error) {
	// TODO: validate version?
	imageName := common.Concat(defaultImageRepositoryAndName, ":", p.Version)
	if p.CustomImageName != "" {
		imageName = p.CustomImageName
	}

	terminationGracePeriodSeconds := defaultTerminationGracePeriodSeconds

	// TODO: quota support
	usersSecret := NewSecretVolume(ElasticUsersSecretName(p.ClusterName), "users")
	probeSecret := NewSelectiveSecretVolumeWithMountPath(
		ElasticInternalUsersSecretName(p.ClusterName), "probe-user",
		probeUserSecretMountPath, []string{probeUser.Name},
	)
	dataVolume := NewDefaultEmptyDirVolume()
	extraFilesSecretVolume := NewSecretVolumeWithMountPath(
		extraFilesRef.Name,
		"extrafiles",
		"/usr/share/elasticsearch/config/extrafiles",
	)

	// TODO: Security Context
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{{
			Env:             NewEnvironmentVars(p, dataVolume, probeUser, extraFilesSecretVolume),
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Name:            containerName,
			Ports:           defaultContainerPorts,
			// TODO: Hardcoded resource limits and requests
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("800m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			ReadinessProbe: &corev1.Probe{
				FailureThreshold:    3,
				InitialDelaySeconds: 10,
				PeriodSeconds:       10,
				SuccessThreshold:    3,
				TimeoutSeconds:      5,
				Handler: corev1.Handler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							defaultReadinessProbeScript,
						},
					},
				},
			},
			VolumeMounts: append(
				initcontainer.SharedVolumes.EsContainerVolumeMounts(),
				dataVolume.VolumeMount(),
				usersSecret.VolumeMount(),
				probeSecret.VolumeMount(),
				extraFilesSecretVolume.VolumeMount(),
			),
		}},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		Volumes: append(
			initcontainer.SharedVolumes.Volumes(),
			dataVolume.Volume(),
			usersSecret.Volume(),
			probeSecret.Volume(),
			extraFilesSecretVolume.Volume(),
		),
	}

	// Setup init containers
	initContainers, err := initcontainer.NewInitContainers(imageName, LinkedFiles, p.SetVMMaxMapCount)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	podSpec.InitContainers = initContainers
	return podSpec, nil
}
