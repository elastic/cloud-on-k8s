// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

func TestNewMergedESConfig(t *testing.T) {
	nodeML := "node.ml"
	xPackSecurityAuthcRealmsActiveDirectoryAD1Order := "xpack.security.authc.realms.active_directory.ad1.order"
	xPackSecurityAuthcRealmsAD1Type := "xpack.security.authc.realms.ad1.type"
	xPackSecurityAuthcRealmsAD1Order := "xpack.security.authc.realms.ad1.order"

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
	}

	policyCfg := common.MustCanonicalConfig(map[string]interface{}{
		esv1.DiscoverySeedProviders: "policy-override",
	})

	tests := []struct {
		name          string
		version       string
		ipFamily      corev1.IPFamily
		cfgData       map[string]interface{}
		policyCfgData *common.CanonicalConfig
		assert        func(cfg CanonicalConfig)
	}{
		{
			name:     "in 6.x, empty config should have the default file and native realm settings configured",
			version:  "6.8.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ShardAwarenessAttributes})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeAttrNodeName})))
			},
		},
		{
			name:     "in 6.x, sample config should have the default file realm settings configured",
			version:  "6.8.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]interface{}{
				nodeML: true,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ShardAwarenessAttributes})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeAttrNodeName})))
			},
		},
		{
			name:     "in 6.x, active_directory realm settings should be merged with the default file and native realm settings",
			version:  "6.8.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]interface{}{
				nodeML:                           true,
				xPackSecurityAuthcRealmsAD1Type:  "active_directory",
				xPackSecurityAuthcRealmsAD1Order: 0,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsAD1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsAD1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ShardAwarenessAttributes})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeAttrNodeName})))
			},
		},
		{
			name:     "in 7.x, empty config should have the default file and native realm settings configured",
			version:  "7.3.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
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
			cfgData: map[string]interface{}{
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
			cfgData: map[string]interface{}{
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
			name:     "in 6.x, seed hosts setting should be discovery.zen.hosts_provider",
			version:  "6.8.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.DiscoveryZenHostsProvider})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.DiscoverySeedProviders})))
			},
		},
		{
			name:     "starting 7.x, seed hosts settings should be discovery.seed_providers",
			version:  "7.0.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.DiscoveryZenHostsProvider})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.DiscoverySeedProviders})))
			},
		},
		{
			name:     "prior to 7.8.1, we should not set allowed license upload types",
			version:  "7.5.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.XPackLicenseUploadTypes})))
			},
		},
		{
			name:     "starting 7.8.1, we should set allowed license upload types",
			version:  "7.8.1",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackLicenseUploadTypes})))
			},
		},
		{
			name:     "prior to 8.2.0 we should not enable the readiness.port",
			version:  "8.1.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.ReadinessPort})))
			},
		},
		{
			name:     "starting 8.2.0 we should enable the readiness.port",
			version:  "8.2.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData:  map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.ReadinessPort})))
			},
		},
		{
			name:     "user-provided Elasticsearch config overrides should have precedence over ECK config",
			version:  "7.6.0",
			ipFamily: corev1.IPv4Protocol,
			cfgData: map[string]interface{}{
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
			cfgData: map[string]interface{}{
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
			name:     "configuration is adjusted for IP family",
			version:  "7.6.0",
			ipFamily: corev1.IPv6Protocol,
			cfgData:  map[string]interface{}{},
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
			cfg, err := NewMergedESConfig("clusterName", ver, tt.ipFamily, commonv1.HTTPConfig{}, commonv1.Config{Data: tt.cfgData}, tt.policyCfgData)
			require.NoError(t, err)
			tt.assert(cfg)
		})
	}
}
