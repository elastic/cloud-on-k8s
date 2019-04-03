// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

// NewDefaultESConfig builds the elasticsearch configuration file from the given parameters
func NewDefaultESConfig(
	clusterName string,
	zenMinMasterNodes int,
	nodeTypes v1alpha1.NodeTypesSpec,
	licenseType v1alpha1.LicenseType,
) FlatConfig {
	return baseConfig(clusterName, zenMinMasterNodes).
		MergeWith(nodeTypesConfig(nodeTypes)).
		MergeWith(SelfGenLicenseConfig(licenseType)).
		MergeWith(XPackSecurityConfig(licenseType))
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string, minMasterNodes int) FlatConfig {
	return FlatConfig{
		// derive node name dynamically from the pod name, injected as env var
		NodeName:    "${" + EnvPodName + "}",
		ClusterName: clusterName,

		DiscoveryZenPingUnicastHosts:   services.DiscoveryServiceName(clusterName),
		DiscoveryZenMinimumMasterNodes: strconv.Itoa(minMasterNodes),

		// derive IP dynamically from the pod IP, injected as env var
		NetworkPublishHost:          "${" + EnvPodIP + "}",
		NetworkHost:                 "0.0.0.0",
		TransportProfilesClientPort: strconv.Itoa(network.TransportClientPort),

		PathData: initcontainer.DataSharedVolume.EsContainerMountPath,
		PathLogs: initcontainer.LogsSharedVolume.EsContainerMountPath,
	}
}

// nodeTypesConfig returns configuration bit related to nodes types
func nodeTypesConfig(nodeTypes v1alpha1.NodeTypesSpec) FlatConfig {
	return FlatConfig{
		NodeMaster: fmt.Sprintf("%t", nodeTypes.Master),
		NodeData:   fmt.Sprintf("%t", nodeTypes.Data),
		NodeIngest: fmt.Sprintf("%t", nodeTypes.Ingest),
		NodeML:     fmt.Sprintf("%t", nodeTypes.ML),
	}
}

func XPackSecurityConfig(licenseType v1alpha1.LicenseType) FlatConfig {
	if licenseType == v1alpha1.LicenseTypeBasic {
		// disable XPack security for basic licenses
		return FlatConfig{
			XPackSecurityEnabled: "false",
		}
	}

	return FlatConfig{
		// x-pack security general settings
		XPackSecurityEnabled:                      "true",
		XPackSecurityAuthcReservedRealmEnabled:    "false",
		XPackSecurityTransportSslVerificationMode: "certificate",

		// client profiles
		TransportProfilesClientXPackSecurityType:                    "client",
		TransportProfilesClientXPackSecuritySslClientAuthentication: "none",

		// x-pack security http settings
		XPackSecurityHttpSslEnabled:                "true",
		XPackSecurityHttpSslKey:                    path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		XPackSecurityHttpSslCertificate:            path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CertFileName),
		XPackSecurityHttpSslCertificateAuthorities: path.Join(volume.NodeCertificatesSecretVolumeMountPath, certificates.CAFileName),

		// x-pack security transport settings
		XPackSecurityTransportSslEnabled:                "true",
		XPackSecurityTransportSslKey:                    path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		XPackSecurityTransportSslCertificate:            path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CertFileName),
		XPackSecurityTransportSslCertificateAuthorities: path.Join(volume.NodeCertificatesSecretVolumeMountPath, certificates.CAFileName),

		// TODO: it would be great if we could move this out of "generic extra files" and into a more scoped secret
		//       alternatively, we could rename extra files to be a bit more specific and make it more of a
		//       reusable component somehow.
		XPackSecurityTransportSslTrustRestrictionsPath: fmt.Sprintf(
			"%s/%s",
			volume.ExtraFilesSecretVolumeMountPath,
			nodecerts.TrustRestrictionsFilename,
		),
	}
}

func DisableXPackSecurity(config FlatConfig) FlatConfig {
	configCopy := FlatConfig{}.MergeWith(config)
	for field := range configCopy {
		if strings.HasPrefix("xpack.security", field) {
			delete(configCopy, field)
		}
	}
	configCopy[XPackSecurityEnabled] = "false"
	return configCopy
}

func SelfGenLicenseConfig(licenseType v1alpha1.LicenseType) FlatConfig {
	switch licenseType {
	case v1alpha1.LicenseTypeBasic:
		return FlatConfig{XPackLicenseSelfGeneratedType: string(v1alpha1.LicenseTypeBasic)}
	case v1alpha1.LicenseTypeTrial:
		return FlatConfig{XPackLicenseSelfGeneratedType: string(v1alpha1.LicenseTypeTrial)}
	default:
		return FlatConfig{}
	}
}

func DisableSelfGenLicense(config FlatConfig) FlatConfig {
	configCopy := FlatConfig{}.MergeWith(config)
	delete(configCopy, XPackLicenseSelfGeneratedType)
	return configCopy
}
