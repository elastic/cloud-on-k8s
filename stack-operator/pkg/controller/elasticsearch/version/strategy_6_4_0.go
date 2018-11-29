package version

import (
	"fmt"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"path"
	"strconv"
	"strings"

	commonv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/initcontainer"
	corev1 "k8s.io/api/core/v1"
)

var (
	// linkedFiles_6_4_0 describe how the user related secrets are mapped into the pod's filesystem.
	linkedFiles_6_4_0 = initcontainer.LinkedFilesArray{
		Array: []initcontainer.LinkedFile{
			{
				Source: common.Concat(support.DefaultSecretMountPath, "/", support.ElasticUsersFile),
				Target: common.Concat("/usr/share/elasticsearch/config", "/", support.ElasticUsersFile),
			},
			{
				Source: common.Concat(support.DefaultSecretMountPath, "/", support.ElasticUsersRolesFile),
				Target: common.Concat("/usr/share/elasticsearch/config", "/", support.ElasticUsersRolesFile),
			},
		},
	}
)

//noinspection GoSnakeCaseUsage
type strategy_6_4_0 struct {
	versionHolder
	versionedNewPodLabels
	lowestHighestSupportedVersions
}

//noinspection GoSnakeCaseUsage
func newStrategy_6_4_0(v version.Version) strategy_6_4_0 {
	strategy := strategy_6_4_0{
		versionHolder:         versionHolder{version: v},
		versionedNewPodLabels: versionedNewPodLabels{version: v},
		lowestHighestSupportedVersions: lowestHighestSupportedVersions{
			// 5.6.0 is the lowest wire compatibility version for 6.x
			lowestSupportedVersion: version.MustParse("5.6.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			highestSupportedVersion: version.MustParse("6.4.99"),
		},
	}
	return strategy
}

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func (s strategy_6_4_0) ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl support.NewPodSpecParams,
) ([]support.PodSpecContext, error) {
	// we mount the elastic users secret over at /secrets, which needs to match the "linkedFiles" in the init-container
	// creation below.
	// TODO: make this association clearer.
	paramsTmpl.UsersSecretVolume = support.NewSecretVolume(
		support.ElasticUsersSecretName(es.Name),
		"users",
	)

	return newExpectedPodSpecs(es, paramsTmpl, s.newEnvironmentVars, s.newInitContainers)
}

// newInitContainers returns a list of init containers
func (s strategy_6_4_0) newInitContainers(
	imageName string,
	keyStoreInit initcontainer.KeystoreInit,
	setVMMaxMapCount bool,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(imageName, linkedFiles_6_4_0, keyStoreInit, setVMMaxMapCount)
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func (s strategy_6_4_0) newEnvironmentVars(
	p support.NewPodSpecParams,
	extraFilesSecretVolume support.SecretVolume,
) []corev1.EnvVar {
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

		{Name: support.EnvPathData, Value: initcontainer.DataSharedVolume.EsContainerMountPath},
		{Name: support.EnvPathLogs, Value: initcontainer.LogsSharedVolume.EsContainerMountPath},

		// TODO: it would be great if we could move this out of "generic extra files" and into a more scoped secret
		//       alternatively, we could rename extra files to be a bit more specific and make it more of a
		//       reusable component somehow.
		{
			Name:  "xpack.security.transport.ssl.trust_restrictions.path",
			Value: fmt.Sprintf("%s/trust.yml", extraFilesSecretVolume.VolumeMount().MountPath),
		},

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: support.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM", heapSize, heapSize)},

		{Name: support.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: support.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: support.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: support.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: support.EnvXPackSecurityEnabled, Value: "true"},
		{Name: support.EnvXPackLicenseSelfGeneratedType, Value: "trial"},
		{Name: support.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: "PROBE_USERNAME", Value: p.ProbeUser.Name},
		{Name: "PROBE_PASSWORD_FILE", Value: path.Join(support.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: "transport.profiles.client.port", Value: strconv.Itoa(support.TransportClientPort)},
	}
}

// NewPod constructs a pod from the given parameters.
func (s strategy_6_4_0) NewPod(
	es v1alpha1.ElasticsearchCluster,
	podSpecCtx support.PodSpecContext,
) (corev1.Pod, error) {
	pod, err := newPod(s, es, podSpecCtx)
	if err != nil {
		return pod, err
	}

	if es.Spec.FeatureFlags.Get(commonv1alpha1.FeatureFlagNodeCertificates).Enabled {
		log.Info("Node certificates feature flag enabled", "pod", pod.Name)
		pod = s.configureNodeCertificates(pod)
	}

	return pod, nil
}

// UpdateDiscovery configures discovery settings based on the given list of pods.
func (s strategy_6_4_0) UpdateDiscovery(esClient *client.Client, allPods []corev1.Pod) error {
	return updateZen1Discovery(esClient, allPods)
}

// configureNodeCertificates configures node certificates for the provided pod
func (s strategy_6_4_0) configureNodeCertificates(pod corev1.Pod) corev1.Pod {
	nodeCertificatesVolume := support.NewSecretVolumeWithMountPath(
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
				Value: "certificate",
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
