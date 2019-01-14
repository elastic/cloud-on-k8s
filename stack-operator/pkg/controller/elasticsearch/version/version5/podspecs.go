package version5

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	corev1 "k8s.io/api/core/v1"
)

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl support.NewPodSpecParams,
	resourcesState support.ResourcesState,
) ([]support.PodSpecContext, error) {
	// we currently mount the users secret volume as the x-pack folder. we cannot symlink these into the existing
	// config/x-pack/ folder because of the Java Security Manager restrictions.
	// in the future we might want to consider bind-mounting specific files instead to be less broad
	paramsTmpl.UsersSecretVolume = support.NewSecretVolumeWithMountPath(
		support.ElasticUsersSecretName(es.Name),
		"users",
		"/usr/share/elasticsearch/config/x-pack",
	)

	// XXX: we need to ensure that a system key is available and used, otherwise connecting with a transport client
	// potentially bypasses x-pack security.

	return version.NewExpectedPodSpecs(es, paramsTmpl, newEnvironmentVars, newInitContainers, newSidecarContainers, []corev1.Volume{})
}

// newInitContainers returns a list of init containers
func newInitContainers(
	imageName string,
	setVMMaxMapCount bool,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(imageName, initcontainer.LinkedFilesArray{}, setVMMaxMapCount)
}

// newSidecarContainers returns a list of sidecar containers.
func newSidecarContainers(
	_ string,
	_ support.NewPodSpecParams,
	_ map[string]support.VolumeLike,
) ([]corev1.Container, error) {
	// TODO keystore is supported as of 5.3. Decide whether we continue supporting 5.x and branch off a 5.3 driver if so.
	return []corev1.Container{}, nil
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
	p support.NewPodSpecParams,
	nodeCertificatesVolume support.SecretVolume,
	extraFilesSecretVolume support.SecretVolume,
) []corev1.EnvVar {
	// TODO: require system key setting for 5.2 and up

	heapSize := version.MemoryLimitsToHeapSize(*p.Resources.Limits.Memory())

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
		{Name: support.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", heapSize, heapSize, version.SecurityPropsFile)},

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
