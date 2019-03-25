// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"errors"
	"fmt"
	"path"
	"strconv"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/sidecar"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
)

var (
	// linkedFiles6 describe how the user related secrets are mapped into the pod's filesystem.
	linkedFiles6 = initcontainer.LinkedFilesArray{
		Array: []initcontainer.LinkedFile{
			{
				Source: stringsutil.Concat(volume.DefaultSecretMountPath, "/", user.ElasticUsersFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", user.ElasticUsersFile),
			},
			{
				Source: stringsutil.Concat(volume.DefaultSecretMountPath, "/", user.ElasticRolesFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", user.ElasticRolesFile),
			},
			{
				Source: stringsutil.Concat(volume.DefaultSecretMountPath, "/", user.ElasticUsersRolesFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", user.ElasticUsersRolesFile),
			},
		},
	}
	sideCarSharedVolume = volume.NewEmptyDirVolume("sidecar-bin", "/opt/sidecar/bin")

	defaultSidecarMemoryLimits = resource.MustParse("64Mi")
	defaultSidecarCPULimits    = resource.MustParse("10m")
)

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func ExpectedPodSpecs(
	es v1alpha1.Elasticsearch,
	paramsTmpl pod.NewPodSpecParams,
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	// we mount the elastic users secret over at /secrets, which needs to match the "linkedFiles" in the init-container
	// creation below.
	// TODO: make this association clearer.
	paramsTmpl.UsersSecretVolume = volume.NewSecretVolume(
		user.ElasticUsersRolesSecretName(es.Name),
		"users",
	)

	return version.NewExpectedPodSpecs(
		es,
		paramsTmpl,
		newEnvironmentVars,
		newInitContainers,
		newSidecarContainers,
		[]corev1.Volume{sideCarSharedVolume.Volume()},
		operatorImage,
	)
}

// newInitContainers returns a list of init containers
func newInitContainers(
	elasticsearchImage string,
	operatorImage string,
	setVMMaxMapCount bool,
	nodeCertificatesVolume volume.SecretVolume,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(
		elasticsearchImage,
		operatorImage,
		linkedFiles6,
		setVMMaxMapCount,
		nodeCertificatesVolume,
		initcontainer.NewSidecarInitContainer(sideCarSharedVolume, operatorImage),
	)
}

// newSidecarContainers returns a list of sidecar containers.
func newSidecarContainers(
	imageName string,
	spec pod.NewPodSpecParams,
	volumes map[string]volume.VolumeLike,
) ([]corev1.Container, error) {

	keystoreVolume, ok := volumes[keystore.SecretVolumeName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("no keystore volume present %v", volumes))
	}
	reloadCredsUser, ok := volumes[volume.ReloadCredsUserVolumeName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("no reload creds user volume present %v", volumes))
	}
	certs, ok := volumes[volume.NodeCertificatesSecretVolumeName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("no node certificates volume present %v", volumes))
	}
	esEndpoint := fmt.Sprintf("%s://127.0.0.1:%d", network.ProtocolForLicense(spec.LicenseType), network.HTTPPort)
	return []corev1.Container{
		{
			Name:            "keystore-updater",
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{path.Join(sideCarSharedVolume.VolumeMount().MountPath, "keystore-updater")},
			Env: []corev1.EnvVar{
				{Name: sidecar.EnvSourceDir, Value: keystoreVolume.VolumeMount().MountPath},
				{Name: sidecar.EnvReloadCredentials, Value: "true"},
				{Name: sidecar.EnvUsername, Value: spec.ReloadCredsUser.Name},
				{Name: sidecar.EnvPasswordFile, Value: path.Join(volume.ReloadCredsUserSecretMountPath, spec.ReloadCredsUser.Name)},
				{Name: sidecar.EnvVersion, Value: spec.Version},
				{Name: sidecar.EnvCertPath, Value: path.Join(certs.VolumeMount().MountPath, certificates.CAFileName)},
				{Name: sidecar.EnvEndpoint, Value: esEndpoint},
			},
			VolumeMounts: append(
				initcontainer.PrepareFsSharedVolumes.EsContainerVolumeMounts(),
				sideCarSharedVolume.VolumeMount(),
				certs.VolumeMount(),
				keystoreVolume.VolumeMount(),
				reloadCredsUser.VolumeMount(),
			),
			Resources: corev1.ResourceRequirements{
				// Requests is not specified to get assigned a qosClass of Guaranteed, like for the Elasticsearch container
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: defaultSidecarMemoryLimits,
					corev1.ResourceCPU:    defaultSidecarCPULimits,
				},
			},
		},
	}, nil
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
	p pod.NewPodSpecParams,
	nodeCertificatesVolume volume.SecretVolume,
	extraFilesSecretVolume volume.SecretVolume,
) []corev1.EnvVar {
	heapSize := version.MemoryLimitsToHeapSize(*p.Resources.Limits.Memory())

	vars := []corev1.EnvVar{
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

		{Name: settings.EnvNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: settings.EnvNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: settings.EnvNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},
		{Name: settings.EnvNodeML, Value: fmt.Sprintf("%t", p.NodeTypes.ML)},

		{Name: settings.EnvReadinessProbeProtocol, Value: network.ProtocolForLicense(p.LicenseType)},
		{Name: settings.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: settings.EnvProbePasswordFile, Value: path.Join(volume.ProbeUserSecretMountPath, p.ProbeUser.Name)},
		{Name: settings.EnvTransportProfilesClientPort, Value: strconv.Itoa(network.TransportClientPort)},
	}

	vars = append(vars, xpackEnvVars(nodeCertificatesVolume, extraFilesSecretVolume, p.LicenseType)...)

	return vars
}

func xpackEnvVars(
	nodeCertificatesVolume volume.SecretVolume,
	extraFilesSecretVolume volume.SecretVolume,
	licenseType v1alpha1.LicenseType,
) []corev1.EnvVar {

	if licenseType == v1alpha1.LicenseTypeBasic {
		// disable x-pack security
		return []corev1.EnvVar{
			{Name: settings.EnvXPackSecurityEnabled, Value: "false"},
		}
	}

	// enable x-pack security, including TLS
	vars := []corev1.EnvVar{
		// x-pack security general settings
		{Name: settings.EnvXPackSecurityEnabled, Value: "true"},
		{Name: settings.EnvXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: settings.EnvXPackSecurityTransportSslVerificationMode, Value: "certificate"},

		// client profiles
		{Name: settings.EnvTransportProfilesClientXPackSecurityType, Value: "client"},
		{Name: settings.EnvTransportProfilesClientXPackSecuritySslClientAuthentication, Value: "none"},

		// x-pack security http settings
		{Name: settings.EnvXPackSecurityHttpSslEnabled, Value: "true"},
		{
			Name:  settings.EnvXPackSecurityHttpSslKey,
			Value: path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		},
		{
			Name:  settings.EnvXPackSecurityHttpSslCertificate,
			Value: path.Join(nodeCertificatesVolume.VolumeMount().MountPath, nodecerts.CertFileName),
		},
		{
			Name:  settings.EnvXPackSecurityHttpSslCertificateAuthorities,
			Value: path.Join(nodeCertificatesVolume.VolumeMount().MountPath, certificates.CAFileName),
		},
		// x-pack security transport settings
		{Name: settings.EnvXPackSecurityTransportSslEnabled, Value: "true"},
		{
			Name:  settings.EnvXPackSecurityTransportSslKey,
			Value: path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		},
		{
			Name:  settings.EnvXPackSecurityTransportSslCertificate,
			Value: path.Join(nodeCertificatesVolume.VolumeMount().MountPath, nodecerts.CertFileName),
		},
		{
			Name:  settings.EnvXPackSecurityTransportSslCertificateAuthorities,
			Value: path.Join(nodeCertificatesVolume.VolumeMount().MountPath, certificates.CAFileName),
		},

		// TODO: it would be great if we could move this out of "generic extra files" and into a more scoped secret
		//       alternatively, we could rename extra files to be a bit more specific and make it more of a
		//       reusable component somehow.
		{
			Name: settings.EnvXPackSecurityTransportSslTrustRestrictionsPath,
			Value: fmt.Sprintf(
				"%s/%s",
				extraFilesSecretVolume.VolumeMount().MountPath,
				nodecerts.TrustRestrictionsFilename,
			),
		},
	}

	if licenseType == v1alpha1.LicenseTypeTrial {
		// auto-generate a trial license
		vars = append(vars, corev1.EnvVar{Name: settings.EnvXPackLicenseSelfGeneratedType, Value: "trial"})
	}

	return vars
}
