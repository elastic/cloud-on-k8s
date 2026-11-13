// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"bytes"
	"math"
	"path"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

func TestNewMergedESConfig(t *testing.T) {
	nodeML := "node.ml"
	xPackSecurityAuthcRealmsActiveDirectoryAD1Order := "xpack.security.authc.realms.active_directory.ad1.order"

	// eckPKIRealmCfg captures the ECK-managed PKI realm settings.
	type eckPKIRealmCfg struct {
		Order                  int      `yaml:"order"`
		CertificateAuthorities []string `yaml:"certificate_authorities"`
	}
	// elasticsearchCfg captures some of the fields we want to validate in these tests
	type elasticsearchCfg struct {
		Discovery struct {
			SeedProviders string `yaml:"seed_providers"`
		} `yaml:"discovery"`
		HTTP struct {
			PublishHost string `yaml:"publish_host"`
		} `yaml:"http"`
		Network struct {
			PublishHost string `yaml:"publish_host"`
		} `yaml:"network"`
		Xpack struct {
			Security struct {
				Authc struct {
					Realms struct {
						Pki struct {
							EckPki *eckPKIRealmCfg `yaml:"eck_managed_pki"`
						} `yaml:"pki"`
					} `yaml:"realms"`
				} `yaml:"authc"`
			} `yaml:"security"`
		} `yaml:"xpack"`
	}
	// eckPKIRealm renders cfg and returns the decoded eck_managed_pki realm section, or nil when absent.
	eckPKIRealm := func(cfg CanonicalConfig) *eckPKIRealmCfg {
		cfgBytes, err := cfg.Render()
		require.NoError(t, err)
		esCfg := &elasticsearchCfg{}
		require.NoError(t, yaml.Unmarshal(cfgBytes, esCfg))
		return esCfg.Xpack.Security.Authc.Realms.Pki.EckPki
	}
	// eckPKIRealmNodeAttribute returns the value of the node attribute ECK sets with the eck_managed_pki realm, or ""
	// when absent.
	eckPKIRealmNodeAttribute := func(cfg CanonicalConfig) string {
		if len(cfg.HasKeys([]string{commonv1.ECKPKIRealmNodeAttributeConfigKey})) == 0 {
			return ""
		}
		val, err := cfg.String(commonv1.ECKPKIRealmNodeAttributeConfigKey)
		require.NoError(t, err)
		return val
	}

	policyCfg := common.MustCanonicalConfig(map[string]any{
		esv1.DiscoverySeedProviders: "policy-override",
	})

	trustBundlePath := path.Join(volume.ClientCertificatesTrustBundleMountPath, certificates.ClientCertificatesTrustBundleFileName)

	tests := []struct {
		name                         string
		version                      string
		ipFamily                     corev1.IPFamily
		httpConfig                   commonv1.HTTPConfigWithClientOptions
		remoteClusterServerEnabled   bool
		remoteClusterClientEnabled   bool
		clusterHasZoneAwareness      bool
		clientAuthenticationRequired bool
		pkiRealmEnabled              bool
		cfgData                      map[string]any
		policyCfgData                *common.CanonicalConfig
		assert                       func(cfg CanonicalConfig)
	}{
		{
			name:     "No remote cluster client or server by default",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				// Remote cluster client configuration.
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_client.ssl.enabled"})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_client.ssl.enabled"})))
				// Remote cluster server configuration.
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.key"})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.certificate"})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.certificate_authorities"})))
			},
		},
		{
			name:                       "Remote cluster client is enabled",
			version:                    "8.15.0",
			ipFamily:                   corev1.IPv4Protocol,
			remoteClusterClientEnabled: true,
			cfgData:                    map[string]any{},
			assert: func(cfg CanonicalConfig) {
				// Remote cluster client configuration.
				require.Equal(t, 1, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_client.ssl.enabled"})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_client.ssl.enabled"})))
				// Remote cluster server configuration.
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.key"})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.certificate"})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.certificate_authorities"})))
			},
		},
		{
			name:                       "Remote cluster server is enabled",
			version:                    "8.15.0",
			ipFamily:                   corev1.IPv4Protocol,
			remoteClusterServerEnabled: true,
			cfgData:                    map[string]any{},
			assert: func(cfg CanonicalConfig) {
				// Remote cluster client configuration.
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_client.ssl.enabled"})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_client.ssl.enabled"})))
				// Remote cluster server configuration.
				require.Equal(t, 1, len(cfg.HasKeys([]string{"remote_cluster_server.enabled"})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{"remote_cluster.publish_host"})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{"remote_cluster.host"})))
				// Remote cluster server TLS configuration.
				require.Equal(t, 1, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.key"})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.certificate"})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{"xpack.security.remote_cluster_server.ssl.certificate_authorities"})))
			},
		},
		{
			name:                    "zone awareness defaults are added when enabled",
			version:                 "8.15.0",
			ipFamily:                corev1.IPv4Protocol,
			clusterHasZoneAwareness: true,
			cfgData:                 map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{"node.attr.zone"})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ShardAwarenessAttributes})))
				awarenessAttr, err := cfg.String(esv1.ShardAwarenessAttributes)
				require.NoError(t, err)
				require.Equal(t, "k8s_node_name,zone", awarenessAttr)
			},
		},
		{
			name:                    "cluster awareness is enabled when any NodeSet has zone awareness",
			version:                 "8.15.0",
			ipFamily:                corev1.IPv4Protocol,
			clusterHasZoneAwareness: true,
			cfgData:                 map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{"node.attr.zone"})))
				awarenessAttr, err := cfg.String(esv1.ShardAwarenessAttributes)
				require.NoError(t, err)
				require.Equal(t, "k8s_node_name,zone", awarenessAttr)
			},
		},
		{
			name:                    "when cluster zone awareness is disabled, shard awareness remains k8s_node_name",
			version:                 "8.15.0",
			ipFamily:                corev1.IPv4Protocol,
			clusterHasZoneAwareness: false,
			cfgData:                 map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{"node.attr.zone"})))
				awarenessAttr, err := cfg.String(esv1.ShardAwarenessAttributes)
				require.NoError(t, err)
				require.Equal(t, "k8s_node_name", awarenessAttr)
			},
		},
		{
			name:                    "policy config override has precedence over zone awareness defaults",
			version:                 "8.15.0",
			ipFamily:                corev1.IPv4Protocol,
			clusterHasZoneAwareness: true,
			cfgData:                 map[string]any{},
			policyCfgData: common.MustCanonicalConfig(map[string]any{
				esv1.ShardAwarenessAttributes: "rack_id",
			}),
			assert: func(cfg CanonicalConfig) {
				awarenessAttr, err := cfg.String(esv1.ShardAwarenessAttributes)
				require.NoError(t, err)
				require.Equal(t, "rack_id", awarenessAttr)
				require.Equal(t, 1, len(cfg.HasKeys([]string{"node.attr.zone"})))
			},
		},
		{
			name:     "in 7.x, empty config should have the default file and native realm settings configured",
			version:  "7.3.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNativeNative1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ShardAwarenessAttributes})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeAttrNodeName})))
			},
		},
		{
			name:     "in 7.x, sample config should have the default file and native realm settings configured",
			version:  "7.3.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]any{
				nodeML: true,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNativeNative1Order})))
			},
		},
		{
			name:     "in 7.x, active_directory realm settings should be merged with the default file and native realm settings",
			version:  "7.3.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]any{
				nodeML: true,
				xPackSecurityAuthcRealmsActiveDirectoryAD1Order: 0,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNativeNative1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsActiveDirectoryAD1Order})))
			},
		},
		{
			name:     "seed hosts settings should be discovery.seed_providers",
			version:  "7.0.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.DiscoverySeedProviders})))
			},
		},
		{
			name:     "prior to 7.8.1, we should not set allowed license upload types",
			version:  "7.5.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.XPackLicenseUploadTypes})))
			},
		},
		{
			name:     "starting 7.8.1, we should set allowed license upload types",
			version:  "7.8.1",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackLicenseUploadTypes})))
			},
		},
		{
			name:     "prior to 8.2.0 we should not enable the readiness.port",
			version:  "8.1.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.ReadinessPort})))
			},
		},
		{
			name:     "starting 8.2.0 we should enable the readiness.port",
			version:  "8.2.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ReadinessPort})))
			},
		},
		{
			name:     "user-provided Elasticsearch config overrides should have precedence over ECK config",
			version:  "7.6.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]any{
				esv1.DiscoverySeedProviders: "something-else",
			},
			assert: func(cfg CanonicalConfig) {
				cfgBytes, err := cfg.Render()
				require.NoError(t, err)
				esCfg := &elasticsearchCfg{}
				require.NoError(t, yaml.Unmarshal(cfgBytes, &esCfg))
				// default config is still there
				require.Equal(t, "${POD_IP}", esCfg.Network.PublishHost)
				require.Equal(t, "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc", esCfg.HTTP.PublishHost)
				// but has been overridden
				require.Equal(t, "something-else", esCfg.Discovery.SeedProviders)
				require.Equal(t, 1, bytes.Count(cfgBytes, []byte("seed_providers:")))
			},
		},
		{
			name:     "Elasticsearch config overrides from policy should have precedence over default config",
			version:  "7.6.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]any{
				esv1.DiscoverySeedProviders: "something-else",
			},
			policyCfgData: policyCfg,
			assert: func(cfg CanonicalConfig) {
				cfgBytes, err := cfg.Render()
				require.NoError(t, err)
				esCfg := &elasticsearchCfg{}
				require.NoError(t, yaml.Unmarshal(cfgBytes, &esCfg))
				// default config is still there
				require.Equal(t, "${POD_IP}", esCfg.Network.PublishHost)
				require.Equal(t, "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc", esCfg.HTTP.PublishHost)
				// but has been overridden
				require.Equal(t, "policy-override", esCfg.Discovery.SeedProviders)
				require.Equal(t, 1, bytes.Count(cfgBytes, []byte("seed_providers:")))
			},
		},
		{
			name:     "client authentication default is injected from spec field",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			httpConfig: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{Authentication: true},
				},
			},
			cfgData: map[string]any{},
			assert: func(cfg CanonicalConfig) {
				val, err := cfg.String(esv1.XPackSecurityHttpSslClientAuthentication)
				require.NoError(t, err)
				require.Equal(t, "required", val)
			},
		},
		{
			name:     "user config overrides spec-injected client authentication default",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			httpConfig: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{Authentication: true},
				},
			},
			cfgData: map[string]any{
				esv1.XPackSecurityHttpSslClientAuthentication: "optional",
			},
			assert: func(cfg CanonicalConfig) {
				val, err := cfg.String(esv1.XPackSecurityHttpSslClientAuthentication)
				require.NoError(t, err)
				require.Equal(t, "optional", val)
			},
		},
		{
			name:     "stack config policy overrides spec-injected client authentication default",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			httpConfig: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{Authentication: true},
				},
			},
			cfgData: map[string]any{},
			policyCfgData: common.MustCanonicalConfig(map[string]any{
				esv1.XPackSecurityHttpSslClientAuthentication: "optional",
			}),
			assert: func(cfg CanonicalConfig) {
				val, err := cfg.String(esv1.XPackSecurityHttpSslClientAuthentication)
				require.NoError(t, err)
				require.Equal(t, "optional", val)
			},
		},
		{
			name:     "trust bundle appended when client authentication required",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			httpConfig: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{Authentication: true},
				},
			},
			clientAuthenticationRequired: true,
			cfgData:                      map[string]any{},
			assert: func(cfg CanonicalConfig) {
				rendered, err := cfg.Render()
				require.NoError(t, err)
				require.Contains(t, string(rendered), trustBundlePath)
			},
		},
		{
			name:     "trust bundle appended alongside user-specified CAs",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			httpConfig: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{Authentication: true},
				},
			},
			clientAuthenticationRequired: true,
			cfgData: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: []string{
					"/usr/share/elasticsearch/config/http-certs/ca.crt",
					"/custom/ca.crt",
				},
			},
			assert: func(cfg CanonicalConfig) {
				rendered, err := cfg.Render()
				require.NoError(t, err)
				renderedStr := string(rendered)
				require.Contains(t, renderedStr, "/custom/ca.crt")
				require.Contains(t, renderedStr, trustBundlePath)
			},
		},
		{
			name:     "trust bundle not appended when client authentication not required",
			version:  "8.15.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				rendered, err := cfg.Render()
				require.NoError(t, err)
				require.NotContains(t, string(rendered), trustBundlePath)
			},
		},
		{
			name:                         "eck_managed_pki realm injected last in the realm chain with the client trust bundle as certificate_authorities when enabled and client authentication required",
			version:                      "9.6.0",
			ipFamily:                     corev1.IPv4Protocol,
			cfgData:                      map[string]any{},
			clientAuthenticationRequired: true,
			pkiRealmEnabled:              true,
			assert: func(cfg CanonicalConfig) {
				realm := eckPKIRealm(cfg)
				require.NotNil(t, realm, "eck_managed_pki realm must be present")
				// The highest order keeps the realm from shadowing token-based user realms such as JWT or Kerberos.
				require.Equal(t, math.MaxInt32, realm.Order)
				// The realm trusts exactly the ECK client trust bundle, not the wider http.ssl CA list.
				require.Equal(t, []string{trustBundlePath}, realm.CertificateAuthorities)
				require.Equal(t, "true", eckPKIRealmNodeAttribute(cfg), "the node attribute must be set with the realm")
			},
		},
		{
			name:                         "eck_managed_pki realm node attribute set by ECK overrides the user config",
			version:                      "9.6.0",
			ipFamily:                     corev1.IPv4Protocol,
			cfgData:                      map[string]any{commonv1.ECKPKIRealmNodeAttributeConfigKey: "false"},
			clientAuthenticationRequired: true,
			pkiRealmEnabled:              true,
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, "true", eckPKIRealmNodeAttribute(cfg))
			},
		},
		{
			name:                         "eck_managed_pki realm not injected when not enabled even though client authentication required",
			version:                      "9.6.0",
			ipFamily:                     corev1.IPv4Protocol,
			cfgData:                      map[string]any{},
			clientAuthenticationRequired: true,
			assert: func(cfg CanonicalConfig) {
				require.Nil(t, eckPKIRealm(cfg), "eck_managed_pki realm must not be present unless spec.auth.pki.enabled is set")
				require.Empty(t, eckPKIRealmNodeAttribute(cfg))
			},
		},
		{
			name:            "eck_managed_pki realm not injected when client authentication not required",
			version:         "9.6.0",
			ipFamily:        corev1.IPv4Protocol,
			cfgData:         map[string]any{},
			pkiRealmEnabled: true,
			assert: func(cfg CanonicalConfig) {
				require.Nil(t, eckPKIRealm(cfg), "eck_managed_pki realm must not be present when client authentication is not required")
				require.Empty(t, eckPKIRealmNodeAttribute(cfg))
			},
		},
		{
			name:                         "eck_managed_pki realm not injected below ES 9.6 even when client authentication required",
			version:                      "9.5.0",
			ipFamily:                     corev1.IPv4Protocol,
			cfgData:                      map[string]any{},
			clientAuthenticationRequired: true,
			pkiRealmEnabled:              true,
			assert: func(cfg CanonicalConfig) {
				require.Nil(t, eckPKIRealm(cfg), "eck_managed_pki realm must not be injected for ES versions below 9.6")
				require.Empty(t, eckPKIRealmNodeAttribute(cfg))
			},
		},
		{
			name:     "configuration is adjusted for IP family",
			version:  "7.6.0",
			ipFamily: corev1.IPv6Protocol,
			cfgData:  map[string]any{},
			assert: func(cfg CanonicalConfig) {
				cfgBytes, err := cfg.Render()
				require.NoError(t, err)
				esCfg := &elasticsearchCfg{}
				require.NoError(t, yaml.Unmarshal(cfgBytes, &esCfg))
				// publish host IP placeholder is bracketed for IPv6
				require.Equal(t, "[${POD_IP}]", esCfg.Network.PublishHost)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, err := version.Parse(tt.version)
			require.NoError(t, err)
			cfg, err := NewMergedESConfig("clusterName", ver, tt.ipFamily, tt.httpConfig, commonv1.Config{Data: tt.cfgData}, tt.policyCfgData, tt.remoteClusterServerEnabled, tt.remoteClusterClientEnabled, tt.clusterHasZoneAwareness, tt.clientAuthenticationRequired, tt.pkiRealmEnabled)
			require.NoError(t, err)
			tt.assert(cfg)
		})
	}
}

func Test_appendClientTrustBundle(t *testing.T) {
	clientTrustBundle := path.Join(volume.ClientCertificatesTrustBundleMountPath, certificates.ClientCertificatesTrustBundleFileName)
	tests := []struct {
		name        string
		cfg         map[string]any
		expectedCAs any
		wantErr     bool
	}{
		{
			name: "pre-populated certificate_authorities as string",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: "/valid/path.crt",
			},
			expectedCAs: []string{
				"/valid/path.crt",
				clientTrustBundle,
			},
		},
		{
			name: "pre-populated certificate_authorities as string with client trust bundle",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: clientTrustBundle,
			},
			expectedCAs: clientTrustBundle,
		},
		{
			name: "pre-populated certificate_authorities as slice",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: []any{
					"/valid/path.crt",
				},
			},
			expectedCAs: []string{
				"/valid/path.crt",
				clientTrustBundle,
			},
		},
		{
			name: "pre-populated certificate_authorities with client trust bundle",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: []any{
					clientTrustBundle,
				},
			},
			expectedCAs: []string{
				clientTrustBundle,
			},
		},
		{
			name: "invalid certificate_authorities entries",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: []any{
					"/some/other/ca.crt",
					1234, // invalid
				},
			},
			wantErr: true,
		},
		{
			name: "invalid certificate_authorities type",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: map[string]any{
					"invalid": "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "nil certificate_authorities",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: nil,
			},
			expectedCAs: []string{
				clientTrustBundle,
			},
		},
		{
			name: "empty certificate_authorities",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslCertificateAuthorities: []string{},
			},
			expectedCAs: []string{
				clientTrustBundle,
			},
		},
		{
			name: "non-existing certificate_authorities",
			cfg:  map[string]any{},
			expectedCAs: []string{
				clientTrustBundle,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := common.MustCanonicalConfig(tt.cfg)
			err := appendClientTrustBundle(cfg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			switch reflect.TypeOf(tt.expectedCAs).Kind() {
			case reflect.String:
				ca, err := cfg.String(esv1.XPackSecurityHttpSslCertificateAuthorities)
				require.NoError(t, err)
				assert.EqualValues(t, tt.expectedCAs, ca)
			case reflect.Array, reflect.Slice:
				var CAs []any
				require.NoError(t, cfg.UnpackChild(esv1.XPackSecurityHttpSslCertificateAuthorities, &CAs))
				assert.ElementsMatch(t, tt.expectedCAs, CAs)
			default:
				t.Fatalf("unexpected type of CAs")
			}
		})
	}
}

func TestHasClientAuthenticationRequired(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  map[string]any
		want bool
	}{
		{
			name: "required returns true",
			cfg:  map[string]any{esv1.XPackSecurityHttpSslClientAuthentication: "required"},
			want: true,
		},
		{
			name: "optional returns false",
			cfg:  map[string]any{esv1.XPackSecurityHttpSslClientAuthentication: "optional"},
			want: false,
		},
		{
			name: "none returns false",
			cfg:  map[string]any{esv1.XPackSecurityHttpSslClientAuthentication: "none"},
			want: false,
		},
		{
			name: "key absent returns false",
			cfg:  map[string]any{},
			want: false,
		},
		{
			name: "required but ssl disabled returns false",
			cfg: map[string]any{
				esv1.XPackSecurityHttpSslClientAuthentication: "required",
				esv1.XPackSecurityHttpSslEnabled:              "false",
			},
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := CanonicalConfig{common.MustCanonicalConfig(tt.cfg)}
			require.Equal(t, tt.want, HasClientAuthenticationRequired(cfg))
		})
	}
}
