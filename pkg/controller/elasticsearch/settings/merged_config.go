// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived  from the given
// parameters.
func NewMergedESConfig(
	clusterName string,
	httpConfig v1alpha1.HTTPConfig,
	userConfig v1alpha1.Config,
) (CanonicalConfig, error) {
	config, err := common.NewCanonicalConfigFrom(userConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}
	err = config.MergeWith(
		baseConfig(clusterName).CanonicalConfig,
		xpackConfig(httpConfig).CanonicalConfig,
	)
	if err != nil {
		return CanonicalConfig{}, err
	}
	return CanonicalConfig{config}, nil
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string) *CanonicalConfig {
	cfg := map[string]interface{}{
		// derive node name dynamically from the pod name, injected as env var
		NodeName:    "${" + EnvPodName + "}",
		ClusterName: clusterName,

		DiscoveryZenHostsProvider: "file",

		// derive IP dynamically from the pod IP, injected as env var
		NetworkPublishHost: "${" + EnvPodIP + "}",
		NetworkHost:        "0.0.0.0",

		PathData: volume.ElasticsearchDataMountPath,
		PathLogs: volume.ElasticsearchLogsMountPath,
	}
	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}

// xpackConfig returns the configuration bit related to XPack settings
func xpackConfig(httpCfg v1alpha1.HTTPConfig) *CanonicalConfig {
	// enable x-pack security, including TLS
	cfg := map[string]interface{}{
		// x-pack security general settings
		XPackSecurityEnabled:                      "true",
		XPackSecurityAuthcReservedRealmEnabled:    "false",
		XPackSecurityTransportSslVerificationMode: "certificate",

		// x-pack security http settings
		XPackSecurityHttpSslEnabled:     httpCfg.TLS.Enabled(),
		XPackSecurityHttpSslKey:         path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
		XPackSecurityHttpSslCertificate: path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),

		// x-pack security transport settings
		XPackSecurityTransportSslEnabled: "true",
		XPackSecurityTransportSslKey: path.Join(
			volume.ConfigVolumeMountPath,
			volume.NodeTransportCertificatePathSegment,
			volume.NodeTransportCertificateKeyFile,
		),
		XPackSecurityTransportSslCertificate: path.Join(
			volume.ConfigVolumeMountPath,
			volume.NodeTransportCertificatePathSegment,
			volume.NodeTransportCertificateCertFile,
		),
		XPackSecurityTransportSslCertificateAuthorities: []string{
			path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CAFileName),
		},
	}
	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}
