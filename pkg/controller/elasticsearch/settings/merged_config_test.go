// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

func TestNewMergedESConfig(t *testing.T) {
	nodeML := "node.ml"
	xPackSecurityAuthcRealmsActiveDirectoryAD1Order := "xpack.security.authc.realms.active_directory.ad1.order"
	xPackSecurityAuthcRealmsAD1Type := "xpack.security.authc.realms.ad1.type"
	xPackSecurityAuthcRealmsAD1Order := "xpack.security.authc.realms.ad1.order"

	tests := []struct {
		name    string
		version string
		cfgData map[string]interface{}
		assert  func(cfg CanonicalConfig)
	}{
		{
			name:    "in 6.x, empty config should have the default file and native realm settings configured",
			version: "6.8.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Order})))
			},
		},
		{
			name:    "in 6.x, sample config should have the default file realm settings configured",
			version: "6.8.0",
			cfgData: map[string]interface{}{
				nodeML: true,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNative1Order})))
			},
		},
		{
			name:    "in 6.x, active_directory realm settings should be merged with the default file and native realm settings",
			version: "6.8.0",
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
			},
		},
		{
			name:    "in 7.x, empty config should have the default file and native realm settings configured",
			version: "7.3.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackSecurityAuthcRealmsNativeNative1Order})))
			},
		},
		{
			name:    "in 7.x, sample config should have the default file and native realm settings configured",
			version: "7.3.0",
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
			name:    "in 7.x, active_directory realm settings should be merged with the default file and native realm settings",
			version: "7.3.0",
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
			name:    "in 6.x, seed hosts setting should be discovery.zen.hosts_provider",
			version: "6.8.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.DiscoveryZenHostsProvider})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.DiscoverySeedProviders})))
			},
		},
		{
			name:    "starting 7.x, seed hosts settings should be discovery.seed_providers",
			version: "7.0.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.DiscoveryZenHostsProvider})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.DiscoverySeedProviders})))
			},
		},
		{
			name:    "prior to 7.6.0, we should not set allowed license upload types",
			version: "7.5.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{esv1.XPackLicenseUploadTypes})))
			},
		},
		{
			name:    "starting 7.6.0, we should set allowed license upload types",
			version: "7.6.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{esv1.XPackLicenseUploadTypes})))
			},
		},
		{
			name:    "user-provided Elasticsearch config overrides should have precedence over ECK config",
			version: "7.6.0",
			cfgData: map[string]interface{}{
				esv1.DiscoverySeedProviders: "something-else",
			},
			assert: func(cfg CanonicalConfig) {
				cfgBytes, err := cfg.Render()
				require.NoError(t, err)
				// default config is still there
				require.True(t, bytes.Contains(cfgBytes, []byte("publish_host: ${POD_IP}")))
				// but has been overridden
				require.True(t, bytes.Contains(cfgBytes, []byte("seed_providers: something-else")))
				require.Equal(t, 1, bytes.Count(cfgBytes, []byte("seed_providers:")))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, err := version.Parse(tt.version)
			require.NoError(t, err)
			cfg, err := NewMergedESConfig(
				"clusterName",
				*ver,
				commonv1.HTTPConfig{},
				commonv1.Config{Data: tt.cfgData},
			)
			require.NoError(t, err)
			tt.assert(cfg)
		})
	}
}
