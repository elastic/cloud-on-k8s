// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"fmt"
	"path"
	"slices"
	"strings"

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

const (
	// the name of the ES attribute indicating the pod's current k8s node
	nodeAttrK8sNodeName = "k8s_node_name"
	// the name of the ES attribute indicating the pod's current zone
	nodeAttrZone = "zone"
	// the format of the environment variable reference
	envVarReferenceFormat = "${%s}"
)

var (
	nodeAttrNodeName = fmt.Sprintf("%s.%s", esv1.NodeAttr, nodeAttrK8sNodeName)
	nodeAttrZoneName = fmt.Sprintf("%s.%s", esv1.NodeAttr, nodeAttrZone)
)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived from the given
// parameters. The user provided config overrides have precedence over the ECK config.
func NewMergedESConfig(
	clusterName string,
	ver version.Version,
	ipFamily corev1.IPFamily,
	httpConfig commonv1.HTTPConfigWithClientOptions,
	userConfig commonv1.Config,
	esConfigFromStackConfigPolicy *common.CanonicalConfig,
	remoteClusterServerEnabled, remoteClusterClientEnabled bool,
	clusterHasZoneAwareness, clientAuthenticationRequired bool,
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

	// When client authentication is enabled, ensure the trust bundle is included in certificate_authorities.
	// This is done after merging to preserve any user-specified CAs (append-only).
	if clientAuthenticationRequired {
		if err := appendClientTrustBundle(config); err != nil {
			return CanonicalConfig{}, err
		}
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
	cfg[esv1.ShardAwarenessAttributes] = fmt.Sprintf("%s,%s", nodeAttrK8sNodeName, nodeAttrZone)
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
func xpackConfig(ver version.Version, httpCfg commonv1.HTTPConfigWithClientOptions, remoteClusterServerEnabled, remoteClusterClientEnabled bool) *CanonicalConfig {
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

	if httpCfg.TLS.Client.Authentication {
		cfg[esv1.XPackSecurityHttpSslClientAuthentication] = "required"
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

// HasClientAuthenticationRequired checks whether the given config has xpack.security.http.ssl.client_authentication set to "required".
func HasClientAuthenticationRequired(cfg CanonicalConfig) bool {
	val, err := cfg.String(esv1.XPackSecurityHttpSslClientAuthentication)
	if err != nil {
		return false
	}
	return val == "required"
}

// appendClientTrustBundle appends the client trust bundle path to the HTTP SSL certificate authorities.
// This preserves any user-specified CAs while ensuring the trust bundle is included.
func appendClientTrustBundle(config *common.CanonicalConfig) error {
	trustBundlePath := path.Join(volume.ClientCertificatesTrustBundleMountPath, certificates.ClientCertificatesTrustBundleFileName)

	// Unpack the config to get the current value
	var cfg map[string]any
	if err := config.Unpack(&cfg); err != nil {
		return fmt.Errorf("failed to unpack config: %w", err)
	}

	var casToMerge []string //nolint:prealloc
	var existingCAs []string
	if existing, ok := getNestedValue(cfg, esv1.XPackSecurityHttpSslCertificateAuthorities); ok {
		switch v := existing.(type) {
		case nil:
			// esv1.XPackSecurityHttpSslCertificateAuthorities doesn't exist
		case string:
			existingCAs = []string{v}
			// If esv1.XPackSecurityHttpSslCertificateAuthorities is set as a single string
			// capture it in casToMerge because MergeWith below will overwrite it.
			casToMerge = []string{v}
		case []string:
			existingCAs = v
		case []any:
			for i, item := range v {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("%s[%d]: expected string, got %T", esv1.XPackSecurityHttpSslCertificateAuthorities, i, item)
				}
				existingCAs = append(existingCAs, s)
			}
		default:
			return fmt.Errorf("%s: expected string or []string, got %T", esv1.XPackSecurityHttpSslCertificateAuthorities, existing)
		}
	}

	// Check if trust bundle path is already present
	if slices.Contains(existingCAs, trustBundlePath) {
		return nil // Already present, nothing to do
	}

	// Append trust bundle path and merge as a new config to handle type conversion properly
	casToMerge = append(casToMerge, trustBundlePath)
	trustBundleConfig, err := common.NewCanonicalConfigFrom(map[string]any{
		esv1.XPackSecurityHttpSslCertificateAuthorities: casToMerge,
	})
	if err != nil {
		return fmt.Errorf("failed to create trust bundle config: %w", err)
	}
	if err := config.MergeWith(trustBundleConfig); err != nil {
		return fmt.Errorf("failed to merge trust bundle config: %w", err)
	}
	return nil
}

// getNestedValue traverses a nested map structure using a dot-separated key path.
func getNestedValue(cfg map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	current := any(cfg)

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
