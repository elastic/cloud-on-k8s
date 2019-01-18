package version6

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/secret"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

var (
	// linkedFiles6 describe how the user related secrets are mapped into the pod's filesystem.
	linkedFiles6 = initcontainer.LinkedFilesArray{
		Array: []initcontainer.LinkedFile{
			{
				Source: common.Concat(volume.DefaultSecretMountPath, "/", secret.ElasticUsersFile),
				Target: common.Concat("/usr/share/elasticsearch/config", "/", secret.ElasticUsersFile),
			},
			{
				Source: common.Concat(volume.DefaultSecretMountPath, "/", secret.ElasticUsersRolesFile),
				Target: common.Concat("/usr/share/elasticsearch/config", "/", secret.ElasticUsersRolesFile),
			},
		},
	}
)

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl pod.NewPodSpecParams,
	resourcesState reconcile.ResourcesState,
) ([]pod.PodSpecContext, error) {
	// we mount the elastic users secret over at /secrets, which needs to match the "linkedFiles" in the init-container
	// creation below.
	// TODO: make this association clearer.
	paramsTmpl.UsersSecretVolume = volume.NewSecretVolume(
		secret.ElasticUsersSecretName(es.Name),
		"users",
	)

	return version.NewExpectedPodSpecs(es, paramsTmpl, newEnvironmentVars, newInitContainers)
}

// newInitContainers returns a list of init containers
func newInitContainers(
	imageName string,
	keyStoreInit initcontainer.KeystoreInit,
	setVMMaxMapCount bool,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(imageName, linkedFiles6, keyStoreInit, setVMMaxMapCount)
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
	p pod.NewPodSpecParams,
	nodeCertificatesVolume volume.SecretVolume,
	extraFilesSecretVolume volume.SecretVolume,
) []corev1.EnvVar {
	heapSize := version.MemoryLimitsToHeapSize(*p.Resources.Limits.Memory())

	return []corev1.EnvVar{
		{Name: settings.EnvNodeName, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
		}},
		{Name: settings.EnvDiscoveryZenPingUnicastHosts, Value: p.DiscoveryServiceName},
		{Name: settings.EnvClusterName, Value: p.ClusterName},
		{Name: settings.EnvDiscoveryZenMinimumMasterNodes, Value: strconv.Itoa(p.DiscoveryZenMinimumMasterNodes)},
		{Name: settings.EnvNetworkHost, Value: "0.0.0.0"},
		{Name: settings.EnvNetworkPublishHost, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
		}},

		{Name: settings.EnvPathData, Value: initcontainer.DataSharedVolume.EsContainerMountPath},
		{Name: settings.EnvPathLogs, Value: initcontainer.LogsSharedVolume.EsContainerMountPath},

		// TODO: it would be great if we could move this out of "generic extra files" and into a more scoped secret
		//       alternatively, we could rename extra files to be a bit more specific and make it more of a
		//       reusable component somehow.
		{
			Name:  settings.EnvXPackSecurityTransportSslTrustRestrictionsPath,
			Value: fmt.Sprintf("%s/trust.yml", extraFilesSecretVolume.VolumeMount().MountPath),
		},

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: settings.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", heapSize, heapSize, version.SecurityPropsFile)},

		{Name: settings.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: settings.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: settings.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: settings.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: settings.EnvXPackSecurityEnabled, Value: "true"},
		{Name: settings.EnvXPackLicenseSelfGeneratedType, Value: "trial"},
		{Name: settings.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: settings.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: settings.EnvProbePasswordFile, Value: path.Join(volume.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: settings.EnvTransportProfilesClientPort, Value: strconv.Itoa(pod.TransportClientPort)},

		{Name: settings.EnvReadinessProbeProtocol, Value: "https"},

		// x-pack security general settings
		{Name: settings.EnvXPackSecurityTransportSslVerificationMode, Value: "certificate"},

		// client profiles
		{Name: settings.EnvTransportProfilesClientXPackSecurityType, Value: "client"},
		{Name: settings.EnvTransportProfilesClientXPackSecuritySslClientAuthentication, Value: "none"},

		// x-pack security http settings
		{Name: settings.EnvXPackSecurityHttpSslEnabled, Value: "true"},
		{
			Name:  settings.EnvXPackSecurityHttpSslKey,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
		},
		{
			Name:  settings.EnvXPackSecurityHttpSslCertificate,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
		},
		{
			Name:  settings.EnvXPackSecurityHttpSslCertificateAuthorities,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
		},
		// x-pack security transport settings
		{Name: settings.EnvXPackSecurityTransportSslEnabled, Value: "true"},
		{
			Name:  settings.EnvXPackSecurityTransportSslKey,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
		},
		{
			Name:  settings.EnvXPackSecurityTransportSslCertificate,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
		},
		{
			Name:  settings.EnvXPackSecurityTransportSslCertificateAuthorities,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
		},
	}
}
