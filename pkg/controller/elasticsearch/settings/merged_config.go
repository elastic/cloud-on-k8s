// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	netutil "github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

// the name of the ES attribute indicating the pod's current k8s node
const nodeAttrK8sNodeName = "k8s_node_name"

var nodeAttrNodeName = fmt.Sprintf("%s.%s", esv1.NodeAttr, nodeAttrK8sNodeName)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived from the given
// parameters. The user provided config overrides have precedence over the ECK config.
func NewMergedESConfig(
	clusterName string,
	ver version.Version,
	ipFamily corev1.IPFamily,
	httpConfig commonv1.HTTPConfig,
	userConfig commonv1.Config,
	esConfigFromStackConfigPolicy *common.CanonicalConfig,
) (CanonicalConfig, error) {
	userCfg, err := common.NewCanonicalConfigFrom(userConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	config := baseConfig(clusterName, ver, ipFamily).CanonicalConfig
	err = config.MergeWith(
		xpackConfig(ver, httpConfig).CanonicalConfig,
		userCfg,
		esConfigFromStackConfigPolicy,
	)
	if err != nil {
		return CanonicalConfig{}, err
	}
	return CanonicalConfig{config}, nil
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string, ver version.Version, ipFamily corev1.IPFamily) *CanonicalConfig {
	cfg := map[string]interface{}{
		// derive node name dynamically from the pod name, injected as env var
		esv1.NodeName:    "${" + EnvPodName + "}",
		esv1.ClusterName: clusterName,

		// use the DNS name as the publish host
		esv1.NetworkPublishHost: netutil.IPLiteralFor("${"+EnvPodIP+"}", ipFamily),
		esv1.HTTPPublishHost:    "${" + EnvPodName + "}.${" + HeadlessServiceName + "}.${" + EnvNamespace + "}.svc",
		esv1.NetworkHost:        "0",

		// allow ES to be aware of k8s node the pod is running on when allocating shards
		esv1.ShardAwarenessAttributes: nodeAttrK8sNodeName,
		nodeAttrNodeName:              "${" + EnvNodeName + "}",

		esv1.PathData: volume.ElasticsearchDataMountPath,
		esv1.PathLogs: volume.ElasticsearchLogsMountPath,
	}

	// seed hosts setting name changed starting ES 7.X
	fileProvider := "file"
	if ver.Major < 7 {
		cfg[esv1.DiscoveryZenHostsProvider] = fileProvider
	} else {
		cfg[esv1.DiscoverySeedProviders] = fileProvider
		// to avoid misleading error messages about the inability to connect to localhost for discovery despite us using
		// file based discovery
		cfg[esv1.DiscoverySeedHosts] = []string{}
	}

	if ver.GTE(esv1.MinReadinessPortVersion) {
		cfg[esv1.ReadinessPort] = "8080"
	}

	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}

// xpackConfig returns the configuration bit related to XPack settings
func xpackConfig(ver version.Version, httpCfg commonv1.HTTPConfig) *CanonicalConfig {
	// enable x-pack security, including TLS
	cfg := map[string]interface{}{
		// x-pack security general settings
		esv1.XPackSecurityEnabled:                      "true",
		esv1.XPackSecurityAuthcReservedRealmEnabled:    "false",
		esv1.XPackSecurityTransportSslVerificationMode: "certificate",

		// x-pack security http settings
		esv1.XPackSecurityHttpSslEnabled:     httpCfg.TLS.Enabled(),
		esv1.XPackSecurityHttpSslKey:         path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
		esv1.XPackSecurityHttpSslCertificate: path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),

		// x-pack security transport settings
		esv1.XPackSecurityTransportSslEnabled: "true",
		esv1.XPackSecurityTransportSslKey: path.Join(
			volume.TransportCertificatesSecretVolumeMountPath,
			"${POD_NAME}."+certificates.KeyFileName,
		),
		esv1.XPackSecurityTransportSslCertificate: path.Join(
			volume.TransportCertificatesSecretVolumeMountPath,
			"${POD_NAME}."+certificates.CertFileName,
		),
		esv1.XPackSecurityTransportSslCertificateAuthorities: []string{
			path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CAFileName),
			path.Join(volume.RemoteCertificateAuthoritiesSecretVolumeMountPath, certificates.CAFileName),
		},
		esv1.XPackSecurityHttpSslCertificateAuthorities: path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.CAFileName),
	}

	// always enable the built-in file and native internal realms for user auth, ordered as first
	if ver.Major < 7 {
		// 6.x syntax
		cfg[esv1.XPackSecurityAuthcRealmsFile1Type] = "file"
		cfg[esv1.XPackSecurityAuthcRealmsFile1Order] = -100
		cfg[esv1.XPackSecurityAuthcRealmsNative1Type] = "native"
		cfg[esv1.XPackSecurityAuthcRealmsNative1Order] = -99
	} else {
		// 7.x syntax
		cfg[esv1.XPackSecurityAuthcRealmsFileFile1Order] = -100
		cfg[esv1.XPackSecurityAuthcRealmsNativeNative1Order] = -99
	}

	if ver.GTE(version.MustParse("7.8.1")) {
		cfg[esv1.XPackLicenseUploadTypes] = []string{
			string(client.ElasticsearchLicenseTypeTrial), string(client.ElasticsearchLicenseTypeEnterprise),
		}
	}

	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}
