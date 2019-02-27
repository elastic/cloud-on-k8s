// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version5

import (
	"fmt"
	"path"
	"strconv"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/secret"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl pod.NewPodSpecParams,
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	// we currently mount the users secret volume as the x-pack folder. we cannot symlink these into the existing
	// config/x-pack/ folder because of the Java Security Manager restrictions.
	// in the future we might want to consider bind-mounting specific files instead to be less broad
	paramsTmpl.UsersSecretVolume = volume.NewSecretVolumeWithMountPath(
		secret.ElasticUsersRolesSecretName(es.Name),
		"users",
		"/usr/share/elasticsearch/config/x-pack",
	)

	// XXX: we need to ensure that a system key is available and used, otherwise connecting with a transport client
	// potentially bypasses x-pack security.

	return version.NewExpectedPodSpecs(es, paramsTmpl, newEnvironmentVars, newInitContainers, newSidecarContainers, []corev1.Volume{}, operatorImage)
}

// newInitContainers returns a list of init containers
func newInitContainers(
	elasticsearchImage string,
	operatorImage string,
	setVMMaxMapCount bool,
	nodeCertificatesVolume volume.SecretVolume,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(elasticsearchImage, operatorImage, initcontainer.LinkedFilesArray{}, setVMMaxMapCount, nodeCertificatesVolume)
}

// newSidecarContainers returns a list of sidecar containers.
func newSidecarContainers(
	_ string,
	_ pod.NewPodSpecParams,
	_ map[string]volume.VolumeLike,
) ([]corev1.Container, error) {
	// TODO keystore is supported as of 5.3. Decide whether we continue supporting 5.x and branch off a 5.3 driver if so.
	return []corev1.Container{}, nil
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
	p pod.NewPodSpecParams,
	nodeCertificatesVolume volume.SecretVolume,
	extraFilesSecretVolume volume.SecretVolume,
) []corev1.EnvVar {
	// TODO: require system key setting for 5.2 and up

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

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: settings.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", heapSize, heapSize, version.SecurityPropsFile)},

		// TODO: dedicated node types support
		{Name: settings.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: settings.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: settings.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: settings.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: settings.EnvXPackSecurityEnabled, Value: "true"},
		{Name: settings.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: settings.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: settings.EnvProbePasswordFile, Value: path.Join(volume.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: settings.EnvTransportProfilesClientPort, Value: strconv.Itoa(pod.TransportClientPort)},

		{Name: settings.EnvReadinessProbeProtocol, Value: "https"},
		// x-pack general settings
		{
			Name:  settings.EnvXPackSslKey,
			Value: path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		},
		{
			Name:  settings.EnvXPackSslCertificate,
			Value: path.Join(nodeCertificatesVolume.VolumeMount().MountPath, nodecerts.CertFileName),
		},
		{
			Name:  settings.EnvXPackSslCertificateAuthorities,
			Value: path.Join(nodeCertificatesVolume.VolumeMount().MountPath, certificates.CAFileName),
		},
		// client profiles
		{Name: settings.EnvTransportProfilesClientXPackSecurityType, Value: "client"},
		{Name: settings.EnvTransportProfilesClientXPackSecuritySslClientAuthentication, Value: "none"},

		// x-pack http settings
		{Name: settings.EnvXPackSecurityHttpSslEnabled, Value: "true"},

		// x-pack transport settings
		{Name: settings.EnvXPackSecurityTransportSslEnabled, Value: "true"},
		{Name: settings.EnvXPackSecurityTransportSslVerificationMode, Value: "certificate"},
	}
}
