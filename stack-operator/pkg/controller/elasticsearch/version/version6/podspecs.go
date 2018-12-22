package version6

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/sidecar"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	corev1 "k8s.io/api/core/v1"
)

var (
	// linkedFiles6 describe how the user related secrets are mapped into the pod's filesystem.
	linkedFiles6 = initcontainer.LinkedFilesArray{
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
	sideCarSharedVolume = support.NewEmptyDirVolume("sidecar-bin", "/opt/sidecar/bin")
)

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl support.NewPodSpecParams,
	resourcesState support.ResourcesState,
) ([]support.PodSpecContext, error) {
	// we mount the elastic users secret over at /secrets, which needs to match the "linkedFiles" in the init-container
	// creation below.
	// TODO: make this association clearer.
	paramsTmpl.UsersSecretVolume = support.NewSecretVolume(
		support.ElasticUsersSecretName(es.Name),
		"users",
	)

	return version.NewExpectedPodSpecs(
		es,
		paramsTmpl,
		newEnvironmentVars,
		newInitContainers,
		newSidecarContainers,
		[]corev1.Volume{sideCarSharedVolume.Volume()},
	)
}

// newInitContainers returns a list of init containers
func newInitContainers(
	imageName string,
	setVMMaxMapCount bool,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(
		imageName,
		linkedFiles6,
		setVMMaxMapCount,
		initcontainer.NewSidecarInitContainer(sideCarSharedVolume),
	)
}

// newSidecarContainers returns a list of sidecar containers.
func newSidecarContainers(
	imageName string,
	spec support.NewPodSpecParams,
	volumes map[string]support.VolumeLike,
) ([]corev1.Container, error) {

	keystoreVolume, ok := volumes[keystore.SecretVolumeName]
	if !ok {
		return []corev1.Container{}, errors.New(fmt.Sprintf("no keystore volume present %v", volumes))
	}
	probeUser, ok := volumes[support.ProbeUserVolumeName]
	if !ok {
		return []corev1.Container{}, errors.New(fmt.Sprintf("no probe user volume present %v", volumes))
	}
	certs := volumes[support.NodeCertificatesSecretVolumeName]
	return []corev1.Container{
		{
			Name:            "keystore-updater",
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{path.Join(sideCarSharedVolume.VolumeMount().MountPath, "keystore-updater")},
			Env: []corev1.EnvVar{
				{Name: sidecar.EnvSourceDir, Value: keystoreVolume.VolumeMount().MountPath},
				{Name: sidecar.EnvReloadCredentials, Value: "true"},
				{Name: sidecar.EnvUsername, Value: spec.ProbeUser.Name},
				{Name: sidecar.EnvPasswordFile, Value: path.Join(support.ProbeUserSecretMountPath, spec.ProbeUser.Name)},
				{Name: sidecar.EnvCertPath, Value: path.Join(certs.VolumeMount().MountPath, nodecerts.SecretCAKey)},
			},
			VolumeMounts: []corev1.VolumeMount{
				sideCarSharedVolume.VolumeMount(),
				certs.VolumeMount(),
				keystoreVolume.VolumeMount(),
				probeUser.VolumeMount(),
			},
		},
	}, nil
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
	p support.NewPodSpecParams,
	nodeCertificatesVolume support.SecretVolume,
	extraFilesSecretVolume support.SecretVolume,
) []corev1.EnvVar {
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

		// TODO: it would be great if we could move this out of "generic extra files" and into a more scoped secret
		//       alternatively, we could rename extra files to be a bit more specific and make it more of a
		//       reusable component somehow.
		{
			Name:  support.EnvXPackSecurityTransportSslTrustRestrictionsPath,
			Value: fmt.Sprintf("%s/trust.yml", extraFilesSecretVolume.VolumeMount().MountPath),
		},

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: support.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", heapSize, heapSize, version.SecurityPropsFile)},

		{Name: support.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: support.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: support.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: support.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: support.EnvXPackSecurityEnabled, Value: "true"},
		{Name: support.EnvXPackLicenseSelfGeneratedType, Value: "trial"},
		{Name: support.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: support.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: support.EnvProbePasswordFile, Value: path.Join(support.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: support.EnvTransportProfilesClientPort, Value: strconv.Itoa(support.TransportClientPort)},

		{Name: support.EnvReadinessProbeProtocol, Value: "https"},

		// x-pack security general settings
		{Name: support.EnvXPackSecurityTransportSslVerificationMode, Value: "certificate"},

		// client profiles
		{Name: support.EnvTransportProfilesClientXPackSecurityType, Value: "client"},
		{Name: support.EnvTransportProfilesClientXPackSecuritySslClientAuthentication, Value: "none"},

		// x-pack security http settings
		{Name: support.EnvXPackSecurityHttpSslEnabled, Value: "true"},
		{
			Name:  support.EnvXPackSecurityHttpSslKey,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
		},
		{
			Name:  support.EnvXPackSecurityHttpSslCertificate,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
		},
		{
			Name:  support.EnvXPackSecurityHttpSslCertificateAuthorities,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
		},
		// x-pack security transport settings
		{Name: support.EnvXPackSecurityTransportSslEnabled, Value: "true"},
		{
			Name:  support.EnvXPackSecurityTransportSslKey,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
		},
		{
			Name:  support.EnvXPackSecurityTransportSslCertificate,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
		},
		{
			Name:  support.EnvXPackSecurityTransportSslCertificateAuthorities,
			Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
		},
	}
}
