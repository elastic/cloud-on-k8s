// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"fmt"
	"path"
	"strconv"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived  from the given
// parameters.
func NewMergedESConfig(
	clusterName string,
	zenMinMasterNodes int,
	userConfig v1alpha1.Config,
	licenseType v1alpha1.LicenseType,
) (*CanonicalConfig, error) {
	config, err := NewCanonicalConfigFrom(userConfig)
	if err != nil {
		return nil, err
	}
	err = config.MergeWith(
		baseConfig(clusterName, zenMinMasterNodes),
		xpackConfig(licenseType),
	)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string, minMasterNodes int) *CanonicalConfig {
	return MustCanonicalConfig(map[string]interface{}{
		// derive node name dynamically from the pod name, injected as env var
		NodeName:    "${" + EnvPodName + "}",
		ClusterName: clusterName,

		DiscoveryZenPingUnicastHosts:   services.DiscoveryServiceName(clusterName),
		DiscoveryZenMinimumMasterNodes: strconv.Itoa(minMasterNodes),

		// derive IP dynamically from the pod IP, injected as env var
		NetworkPublishHost: "${" + EnvPodIP + "}",
		NetworkHost:        "0.0.0.0",

		PathData: initcontainer.DataSharedVolume.EsContainerMountPath,
		PathLogs: initcontainer.LogsSharedVolume.EsContainerMountPath,
	})
}

// xpackConfig returns the configuration bit related to XPack settings
func xpackConfig(licenseType v1alpha1.LicenseType) *CanonicalConfig {

	// disable x-pack security if using basic
	if licenseType == v1alpha1.LicenseTypeBasic {
		return MustCanonicalConfig(map[string]interface{}{
			XPackSecurityEnabled: "false",
		})
	}

	// enable x-pack security, including TLS
	cfg := map[string]interface{}{
		// x-pack security general settings
		XPackSecurityEnabled:                      "true",
		XPackSecurityAuthcReservedRealmEnabled:    "false",
		XPackSecurityTransportSslVerificationMode: "certificate",

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

	if licenseType == v1alpha1.LicenseTypeTrial {
		// auto-generate a trial license
		cfg[XPackLicenseSelfGeneratedType] = "trial"
	}

	return MustCanonicalConfig(cfg)
}
