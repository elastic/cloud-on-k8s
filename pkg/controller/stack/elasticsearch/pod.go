package elasticsearch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/pkg/controller/stack/common/nodecerts"

	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/initcontainer"
	"github.com/mitchellh/hashstructure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// NewPod constructs a pod from the Stack definition.
func NewPod(s deploymentsv1alpha1.Stack, probeUser client.User, extraFilesRef types.NamespacedName) (corev1.Pod, error) {
	podSpec, err := NewPodSpec(BuildNewPodSpecParams(s), probeUser, extraFilesRef)
	if err != nil {
		return corev1.Pod{}, err
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NewNodeName(s.Name),
			Namespace: s.Namespace,
			Labels:    NewLabels(s, true),
		},
		Spec: podSpec,
	}

	if s.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		log.Info(
			fmt.Sprintf(
				"Node certificates feature flag enabled, so injecting certificate volume into container for pod: %s",
				pod.Name,
			),
		)

		nodeCertificatesVolume := NewSecretVolumeWithMountPath(
			"node-certificates",
			"/usr/share/elasticsearch/config/node-certs",
			nodecerts.NodeCertificateSecretObjectKeyForPod(pod).Name,
		)

		podSpec.Volumes = append(podSpec.Volumes, nodeCertificatesVolume.Volume())

		for i, container := range podSpec.InitContainers {
			podSpec.InitContainers[i].VolumeMounts =
				append(container.VolumeMounts, nodeCertificatesVolume.VolumeMount())
		}

		for i, container := range podSpec.Containers {
			podSpec.Containers[i].VolumeMounts = append(container.VolumeMounts, nodeCertificatesVolume.VolumeMount())

			for _, proto := range []string{"http", "transport"} {
				podSpec.Containers[i].Env = append(podSpec.Containers[i].Env,
					corev1.EnvVar{
						Name:  fmt.Sprintf("xpack.security.%s.ssl.enabled", proto),
						Value: "true",
					},
					corev1.EnvVar{
						Name:  fmt.Sprintf("xpack.security.%s.ssl.key", proto),
						Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
					},
					corev1.EnvVar{
						Name:  fmt.Sprintf("xpack.security.%s.ssl.certificate", proto),
						Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
					},
					corev1.EnvVar{
						Name:  fmt.Sprintf("xpack.security.%s.ssl.certificate_authorities", proto),
						Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
					},
				)
			}

			podSpec.Containers[i].Env = append(podSpec.Containers[i].Env,
				corev1.EnvVar{
					Name:  "xpack.security.transport.ssl.verification_mode",
					Value: "none",
				},
				corev1.EnvVar{Name: "READINESS_PROBE_PROTOCOL", Value: "https"},

				// client profiles
				corev1.EnvVar{Name: "transport.profiles.client.xpack.security.type", Value: "client"},
				corev1.EnvVar{Name: "transport.profiles.client.xpack.security.ssl.client_authentication", Value: "none"},
			)

		}

		pod.Spec = podSpec
	}

	return pod, nil
}

// BuildNewPodSpecParams creates a NewPodSpecParams from a Stack definition.
func BuildNewPodSpecParams(s deploymentsv1alpha1.Stack) NewPodSpecParams {
	return NewPodSpecParams{
		Version:                        s.Spec.Version,
		CustomImageName:                s.Spec.Elasticsearch.Image,
		ClusterName:                    s.Name,
		DiscoveryZenMinimumMasterNodes: ComputeMinimumMasterNodes(int(s.Spec.Elasticsearch.NodeCount())),
		DiscoveryServiceName:           DiscoveryServiceName(s.Name),
		SetVMMaxMapCount:               s.Spec.Elasticsearch.SetVMMaxMapCount,
	}
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
	dataVolume := NewDefaultEmptyDirVolume()
	extraFilesSecretVolume := NewSecretVolumeWithMountPath(
		"extrafiles",
		"/usr/share/elasticsearch/config/extrafiles",
		extraFilesRef.Name,
	)

	// TODO: Security Context
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{{
			Env: []corev1.EnvVar{
				{Name: "node.name", Value: "", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
				}},
				{Name: "discovery.zen.ping.unicast.hosts", Value: p.DiscoveryServiceName},
				{Name: "cluster.name", Value: p.ClusterName},
				{Name: "discovery.zen.minimum_master_nodes", Value: strconv.Itoa(p.DiscoveryZenMinimumMasterNodes)},
				{Name: "network.host", Value: "0.0.0.0"},
				{Name: "network.publish_host", Value: "", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				}},

				{Name: "path.data", Value: dataVolume.DataPath()},
				{Name: "path.logs", Value: dataVolume.LogsPath()},
				{
					Name:  "xpack.security.transport.ssl.trust_restrictions.path",
					Value: fmt.Sprintf("%s/trust.yml", extraFilesSecretVolume.VolumeMount().MountPath),
				},
				// TODO: the JVM options are hardcoded, but should be configurable
				{Name: "ES_JAVA_OPTS", Value: "-Xms1g -Xmx1g"},

				// TODO: dedicated node types support
				{Name: "node.master", Value: "true"},
				{Name: "node.data", Value: "true"},
				{Name: "node.ingest", Value: "true"},

				{Name: "xpack.security.enabled", Value: "true"},
				{Name: "xpack.license.self_generated.type", Value: "trial"},
				{Name: "xpack.security.authc.reserved_realm.enabled", Value: "false"},
				{Name: "PROBE_USERNAME", Value: probeUser.Name},
				{Name: "PROBE_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: ElasticInternalUsersSecretName(p.ClusterName),
						},
						Key: probeUser.Name,
					},
				}},
				{Name: "transport.profiles.client.port", Value: strconv.Itoa(TransportClientPort)},
			},
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Name:            "elasticsearch",
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
				extraFilesSecretVolume.VolumeMount(),
			),
		}},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		Volumes: append(
			initcontainer.SharedVolumes.Volumes(),
			dataVolume.Volume(),
			usersSecret.Volume(),
			extraFilesSecretVolume.Volume(),
		),
	}

	initContainer, err := initcontainer.NewInitContainer(imageName, p.SetVMMaxMapCount, LinkedFiles)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	podSpec.InitContainers = append(podSpec.InitContainers, initContainer)

	return podSpec, nil
}
