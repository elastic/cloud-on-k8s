// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"path"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	escerts "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived from the given
// parameters.
func NewMergedESConfig(
	clusterName string,
	ver version.Version,
	httpConfig commonv1beta1.HTTPConfig,
	userConfig commonv1beta1.Config,
	certResources *escerts.CertificateResources,
) (CanonicalConfig, error) {
	config, err := common.NewCanonicalConfigFrom(userConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}
	err = config.MergeWith(
		baseConfig(clusterName, ver).CanonicalConfig,
		xpackConfig(ver, httpConfig, certResources).CanonicalConfig,
	)
	if err != nil {
		return CanonicalConfig{}, err
	}
	return CanonicalConfig{config}, nil
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string, ver version.Version) *CanonicalConfig {
	cfg := map[string]interface{}{
		// derive node name dynamically from the pod name, injected as env var
		estype.NodeName:    "${" + EnvPodName + "}",
		estype.ClusterName: clusterName,

		// derive IP dynamically from the pod IP, injected as env var
		estype.NetworkPublishHost: "${" + EnvPodIP + "}",
		estype.NetworkHost:        "0.0.0.0",

		estype.PathData: volume.ElasticsearchDataMountPath,
		estype.PathLogs: volume.ElasticsearchLogsMountPath,
	}

	// seed hosts setting name changed starting ES 7.X
	fileProvider := "file"
	if ver.Major < 7 {
		cfg[estype.DiscoveryZenHostsProvider] = fileProvider
	} else {
		cfg[estype.DiscoverySeedProviders] = fileProvider
	}

	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}

// xpackConfig returns the configuration bit related to XPack settings
func xpackConfig(ver version.Version, httpCfg commonv1beta1.HTTPConfig, certResources *escerts.CertificateResources) *CanonicalConfig {
	// enable x-pack security, including TLS
	cfg := map[string]interface{}{
		// x-pack security general settings
		estype.XPackSecurityEnabled:                      "true",
		estype.XPackSecurityAuthcReservedRealmEnabled:    "false",
		estype.XPackSecurityTransportSslVerificationMode: "certificate",

		// x-pack security http settings
		estype.XPackSecurityHttpSslEnabled:     httpCfg.TLS.Enabled(),
		estype.XPackSecurityHttpSslKey:         path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
		estype.XPackSecurityHttpSslCertificate: path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),

		// x-pack security transport settings
		estype.XPackSecurityTransportSslEnabled: "true",
		estype.XPackSecurityTransportSslKey: path.Join(
			volume.ConfigVolumeMountPath,
			volume.NodeTransportCertificatePathSegment,
			volume.NodeTransportCertificateKeyFile,
		),
		estype.XPackSecurityTransportSslCertificate: path.Join(
			volume.ConfigVolumeMountPath,
			volume.NodeTransportCertificatePathSegment,
			volume.NodeTransportCertificateCertFile,
		),
		estype.XPackSecurityTransportSslCertificateAuthorities: []string{
			path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CAFileName),
		},
	}

	if certResources.HTTPCACertProvided {
		cfg[estype.XPackSecurityHttpSslCertificateAuthorities] = path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.CAFileName)
	}

	// always enable the built-in file internal realm for user auth, ordered as first
	if ver.Major < 7 {
		// 6.x syntax
		cfg[estype.XPackSecurityAuthcRealmsFile1Type] = "file"
		cfg[estype.XPackSecurityAuthcRealmsFile1Order] = -100
	} else {
		// 7.x syntax
		cfg[estype.XPackSecurityAuthcRealmsFileFile1Order] = -100
	}

	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}
