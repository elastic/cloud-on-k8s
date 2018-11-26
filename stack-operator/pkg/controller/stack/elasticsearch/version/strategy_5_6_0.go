package version

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/client"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/initcontainer"
	corev1 "k8s.io/api/core/v1"
)

//noinspection GoSnakeCaseUsage
type strategy_5_6_0 struct {
	versionHolder
	versionedNewPodLabels
	lowestHighestSupportedVersions
}

//noinspection GoSnakeCaseUsage
func newStrategy_5_6_0(v version.Version) strategy_5_6_0 {
	strategy := strategy_5_6_0{
		versionHolder:         versionHolder{version: v},
		versionedNewPodLabels: versionedNewPodLabels{version: v},
		lowestHighestSupportedVersions: lowestHighestSupportedVersions{
			// TODO: verify that we actually support down to 5.0.0
			// TODO: this follows ES version compat, which is wrong, because we would have to be able to support
			//       an elasticsearch cluster full of 2.x (2.4.6 at least) instances which we would probably only want
			// 		 to do upgrade checks on, snapshot, then terminate + snapshot restore (or re-use volumes).
			lowestSupportedVersion: version.MustParse("5.0.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			highestSupportedVersion: version.MustParse("5.6.99"),
		},
	}
	return strategy
}

// NewExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the stack
func (s strategy_5_6_0) NewExpectedPodSpecs(
	stack v1alpha1.Stack,
	paramsTmpl elasticsearch.NewPodSpecParams,
) ([]elasticsearch.PodSpecContext, error) {
	// we currently mount the users secret volume as the x-pack folder. we cannot symlink these into the existing
	// config/x-pack/ folder because of the Java Security Manager restrictions.
	// in the future we might want to consider bind-mounting specific files instead to be less broad
	paramsTmpl.UsersSecretVolume = elasticsearch.NewSecretVolumeWithMountPath(
		elasticsearch.ElasticUsersSecretName(stack.Name),
		"users",
		"/usr/share/elasticsearch/config/x-pack",
	)

	// XXX: we need to ensure that a system key is available and used, otherwise connecting with a transport client
	// potentially bypasses x-pack security.

	return newExpectedPodSpecs(stack, paramsTmpl, s.newEnvironmentVars, s.newInitContainers)
}

// newInitContainers returns a list of init containers
func (s strategy_5_6_0) newInitContainers(
	imageName string,
	keyStoreInit initcontainer.KeystoreInit,
	setVMMaxMapCount bool,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(imageName, initcontainer.LinkedFilesArray{}, keyStoreInit, setVMMaxMapCount)
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func (s strategy_5_6_0) newEnvironmentVars(
	p elasticsearch.NewPodSpecParams,
	extraFilesSecretVolume elasticsearch.SecretVolume,
) []corev1.EnvVar {
	// TODO: require system key setting for 5.2 and up

	heapSize := memoryLimitsToHeapSize(*p.Resources.Limits.Memory())

	return []corev1.EnvVar{
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

		{Name: elasticsearch.EnvPathData, Value: initcontainer.DataSharedVolume.EsContainerMountPath},
		{Name: elasticsearch.EnvPathLogs, Value: initcontainer.LogsSharedVolume.EsContainerMountPath},

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: elasticsearch.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM", heapSize, heapSize)},

		// TODO: dedicated node types support
		{Name: elasticsearch.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: elasticsearch.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: elasticsearch.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: elasticsearch.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: elasticsearch.EnvXPackSecurityEnabled, Value: "true"},
		{Name: elasticsearch.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: "PROBE_USERNAME", Value: p.ProbeUser.Name},
		{Name: "PROBE_PASSWORD_FILE", Value: path.Join(elasticsearch.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: "transport.profiles.client.port", Value: strconv.Itoa(elasticsearch.TransportClientPort)},
	}
}

// NewPod creates a new pod from the given parameters.
func (s strategy_5_6_0) NewPod(
	stack v1alpha1.Stack,
	podSpecCtx elasticsearch.PodSpecContext,
) (corev1.Pod, error) {
	pod, err := newPod(s, stack, podSpecCtx)
	if err != nil {
		return pod, err
	}

	if stack.Spec.FeatureFlags.Get(v1alpha1.FeatureFlagNodeCertificates).Enabled {
		log.Info("Node certificates feature flag enabled", "pod", pod.Name)
		pod = elasticsearch.ConfigureNodeCertificates(pod)
	}

	return pod, nil
}

// UpdateDiscovery configures discovery settings based on the given list of pods.
func (s strategy_5_6_0) UpdateDiscovery(esClient *client.Client, allPods []corev1.Pod) error {
	return updateZen1Discovery(esClient, allPods)
}

// configureNodeCertificates configures node certificates for the provided pod
func (s strategy_5_6_0) configureNodeCertificates(pod corev1.Pod) corev1.Pod {
	nodeCertificatesVolume := elasticsearch.NewSecretVolumeWithMountPath(
		nodecerts.NodeCertificateSecretObjectKeyForPod(pod).Name,
		"node-certificates",
		"/usr/share/elasticsearch/config/node-certs",
	)
	podSpec := pod.Spec

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
			)
		}

		podSpec.Containers[i].Env = append(podSpec.Containers[i].Env,
			corev1.EnvVar{
				Name:  "xpack.security.transport.ssl.verification_mode",
				Value: "certificate",
			},
			corev1.EnvVar{
				Name:  "xpack.ssl.key",
				Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
			},
			corev1.EnvVar{
				Name:  "xpack.ssl.certificate",
				Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
			},
			corev1.EnvVar{
				Name:  "xpack.ssl.certificate_authorities",
				Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
			},
			corev1.EnvVar{Name: "READINESS_PROBE_PROTOCOL", Value: "https"},

			// client profiles
			corev1.EnvVar{Name: "transport.profiles.client.xpack.security.type", Value: "client"},
			corev1.EnvVar{Name: "transport.profiles.client.xpack.security.ssl.client_authentication", Value: "none"},
		)

	}
	pod.Spec = podSpec

	return pod
}
