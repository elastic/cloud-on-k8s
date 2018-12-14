package version

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/initcontainer"
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

// ExpectedConfigMaps returns a config map that is expected to exist when the Elasticsearch pods are created.
func (s strategy_5_6_0) ExpectedConfigMap(es v1alpha1.ElasticsearchCluster) corev1.ConfigMap {
	return newDefaultConfigMap(es)
}

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func (s strategy_5_6_0) ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl support.NewPodSpecParams,
) ([]support.PodSpecContext, error) {
	// we currently mount the users secret volume as the x-pack folder. we cannot symlink these into the existing
	// config/x-pack/ folder because of the Java Security Manager restrictions.
	// in the future we might want to consider bind-mounting specific files instead to be less broad
	paramsTmpl.UsersSecretVolume = support.NewSecretVolumeWithMountPath(
		support.ElasticUsersSecretName(es.Name),
		"users",
		"/usr/share/elasticsearch/config/x-pack",
	)

	paramsTmpl.ConfigMapVolume = support.NewConfigMapVolume(s.ExpectedConfigMap(es).Name, support.ManagedConfigPath)

	// XXX: we need to ensure that a system key is available and used, otherwise connecting with a transport client
	// potentially bypasses x-pack security.

	return newExpectedPodSpecs(es, paramsTmpl, s.newEnvironmentVars, s.newInitContainers)
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
	p support.NewPodSpecParams,
	nodeCertificatesVolume support.SecretVolume,
	extraFilesSecretVolume support.SecretVolume,
) []corev1.EnvVar {
	// TODO: require system key setting for 5.2 and up

	heapSize := memoryLimitsToHeapSize(*p.Resources.Limits.Memory())

	return []corev1.EnvVar{
		{Name: support.EnvNodeName, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
		}},
		{Name: support.EnvDiscoveryZenPingUnicastHosts, Value: p.DiscoveryServiceName},
		{Name: support.EnvClusterName, Value: p.ClusterName},
		{Name: support.EnvDiscoveryZenMinimumMasterNodes, Value: strconv.Itoa(p.DiscoveryZenMinimumMasterNodes)},
		{Name: support.EnvNetworkHost, Value: "0.0.0.0"},
		{Name: support.EnvNetworkPublishHost, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
		}},

		{Name: support.EnvPathData, Value: initcontainer.DataSharedVolume.EsContainerMountPath},
		{Name: support.EnvPathLogs, Value: initcontainer.LogsSharedVolume.EsContainerMountPath},

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: support.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", heapSize, heapSize, securityPropsFile)},

		// TODO: dedicated node types support
		{Name: support.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: support.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: support.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: support.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: support.EnvXPackSecurityEnabled, Value: "true"},
		{Name: support.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: support.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: support.EnvProbePasswordFile, Value: path.Join(support.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: support.EnvTransportProfilesClientPort, Value: strconv.Itoa(support.TransportClientPort)},

		{Name: support.EnvReadinessProbeProtocol, Value: "https"},
		// x-pack general settings
		{
			Name:  support.EnvXPackSslKey,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
		},
		{
			Name:  support.EnvXPackSslCertificate,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
		},
		{
			Name:  support.EnvXPackSslCertificateAuthorities,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
		},
		// client profiles
		{Name: support.EnvTransportProfilesClientXPackSecurityType, Value: "client"},
		{Name: support.EnvTransportProfilesClientXPackSecuritySslClientAuthentication, Value: "none"},

		// x-pack http settings
		{Name: support.EnvXPackSecurityHttpSslEnabled, Value: "true"},

		// x-pack transport settings
		{Name: support.EnvXPackSecurityTransportSslEnabled, Value: "true"},
		{Name: support.EnvXPackSecurityTransportSslVerificationMode, Value: "certificate"},
	}
}

// NewPod creates a new pod from the given parameters.
func (s strategy_5_6_0) NewPod(
	es v1alpha1.ElasticsearchCluster,
	podSpecCtx support.PodSpecContext,
) (corev1.Pod, error) {
	pod, err := newPod(s, es, podSpecCtx)
	if err != nil {
		return pod, err
	}

	return pod, nil
}

// UpdateDiscovery configures discovery settings based on the given list of pods.
func (s strategy_5_6_0) UpdateDiscovery(esClient *client.Client, allPods []corev1.Pod) error {
	return updateZen1Discovery(esClient, allPods)
}
