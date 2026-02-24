// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	netutil "github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

// the name of the ES attribute indicating the pod's current k8s node
const nodeAttrK8sNodeName = "k8s_node_name"

var nodeAttrNodeName = fmt.Sprintf("%s.%s", esv1.NodeAttr, nodeAttrK8sNodeName)

const nodeAttrZone = "zone"

var nodeAttrZoneName = fmt.Sprintf("%s.%s", esv1.NodeAttr, nodeAttrZone)

const envVarReferenceFormat = "${%s}"

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived from the given
// parameters. The user provided config overrides have precedence over the ECK config.
func NewMergedESConfig(
	clusterName string,
	ver version.Version,
	ipFamily corev1.IPFamily,
	httpConfig commonv1.HTTPConfig,
	userConfig commonv1.Config,
	esConfigFromStackConfigPolicy *common.CanonicalConfig,
	remoteClusterServerEnabled, remoteClusterClientEnabled bool,
	clusterHasZoneAwareness bool,
) (CanonicalConfig, error) {
	userCfg, err := common.NewCanonicalConfigFrom(userConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	config := baseConfig(clusterName, ver, ipFamily, remoteClusterServerEnabled).CanonicalConfig
	err = config.MergeWith(
		zoneAwarenessConfig(clusterHasZoneAwareness).CanonicalConfig,
		xpackConfig(ver, httpConfig, remoteClusterServerEnabled, remoteClusterClientEnabled).CanonicalConfig,
		userCfg,
		esConfigFromStackConfigPolicy,
	)
	if err != nil {
		return CanonicalConfig{}, err
	}
	return CanonicalConfig{config}, nil
}

// zoneAwarenessConfig returns the ES configuration related to zone awareness.
// *Note* in the case of zone awareness being enabled for nodeSets within the cluster, but one or more nodeSets
// do not have the zoneAwareness field configured, we will still add the following configuration for consistency:
//
// - node.attr.zone: ${ZONE}
// - cluster.routing.allocation.awareness.attributes: k8s_node_name,zone
//
// In this case, ${ZONE} will be set to the either the first topology key defined for other nodeSets, or fallback
// to the default topology key [topology.kubernetes.io/zone].
func zoneAwarenessConfig(clusterHasZoneAwareness bool) *CanonicalConfig {
	if !clusterHasZoneAwareness {
		cfg := NewCanonicalConfig()
		return &cfg
	}
	cfg := map[string]any{}
	zoneEnvVarRef := fmt.Sprintf(envVarReferenceFormat, EnvZone)
	cfg[nodeAttrZoneName] = zoneEnvVarRef
	if clusterHasZoneAwareness {
		cfg[esv1.ShardAwarenessAttributes] = fmt.Sprintf("%s,%s", nodeAttrK8sNodeName, nodeAttrZone)
	}
	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string, ver version.Version, ipFamily corev1.IPFamily, remoteClusterServerEnabled bool) *CanonicalConfig {
	cfg := map[string]any{
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

	if remoteClusterServerEnabled {
		cfg[esv1.RemoteClusterEnabled] = "true"
		cfg[esv1.RemoteClusterPublishHost] = "${" + EnvPodName + "}.${" + HeadlessServiceName + "}.${" + EnvNamespace + "}.svc"
		cfg[esv1.RemoteClusterHost] = "0"
	}

	cfg[esv1.DiscoverySeedProviders] = "file"
	// to avoid misleading error messages about the inability to connect to localhost for discovery despite us using
	// file based discovery
	cfg[esv1.DiscoverySeedHosts] = []string{}

	if ver.GTE(esv1.MinReadinessPortVersion) {
		cfg[esv1.ReadinessPort] = "8080"
	}

	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}

// xpackConfig returns the configuration bit related to XPack settings
func xpackConfig(ver version.Version, httpCfg commonv1.HTTPConfig, remoteClusterServerEnabled, remoteClusterClientEnabled bool) *CanonicalConfig {
	// enable x-pack security, including TLS
	cfg := map[string]any{
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

	if remoteClusterServerEnabled {
		cfg[esv1.XPackSecurityRemoteClusterServerSslKey] = path.Join(
			volume.TransportCertificatesSecretVolumeMountPath,
			"${POD_NAME}."+certificates.KeyFileName,
		)
		cfg[esv1.XPackSecurityRemoteClusterServerSslCertificate] = path.Join(
			volume.TransportCertificatesSecretVolumeMountPath,
			"${POD_NAME}."+certificates.CertFileName,
		)
		cfg[esv1.XPackSecurityRemoteClusterServerSslCertificateAuthorities] = []string{
			path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CAFileName),
			path.Join(volume.RemoteCertificateAuthoritiesSecretVolumeMountPath, certificates.CAFileName),
		}
	}

	if remoteClusterClientEnabled {
		cfg[esv1.XPackSecurityRemoteClusterClientSslKey] = true
		cfg[esv1.XPackSecurityRemoteClusterClientSslCertificateAuthorities] = []string{
			// Include /usr/share/elasticsearch/config/transport-certs/ca.crt to trust any additional CA in transport.tls.certificateAuthorities
			path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CAFileName),
			path.Join(volume.RemoteCertificateAuthoritiesSecretVolumeMountPath, certificates.CAFileName),
		}
	}

	// always enable the built-in file and native internal realms for user auth, ordered as first
	cfg[esv1.XPackSecurityAuthcRealmsFileFile1Order] = -100
	cfg[esv1.XPackSecurityAuthcRealmsNativeNative1Order] = -99

	if ver.GTE(version.MustParse("7.8.1")) {
		cfg[esv1.XPackLicenseUploadTypes] = []string{
			string(client.ElasticsearchLicenseTypeTrial), string(client.ElasticsearchLicenseTypeEnterprise),
		}
	}

	return &CanonicalConfig{common.MustCanonicalConfig(cfg)}
}
