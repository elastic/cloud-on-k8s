// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates"
	"github.com/stretchr/testify/require"
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
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNative1Order})))
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
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNative1Order})))
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
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNative1Order})))
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
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNativeNative1Order})))
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
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNativeNative1Order})))
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
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsNativeNative1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsActiveDirectoryAD1Order})))
			},
		},
		{
			name:    "in 6.x, seed hosts setting should be discovery.zen.hosts_provider",
			version: "6.8.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{DiscoveryZenHostsProvider})))
				require.Equal(t, 0, len(cfg.HasKeys([]string{DiscoverySeedProviders})))
			},
		},
		{
			name:    "starting 7.x, seed hosts settings should be discovery.seed_providers",
			version: "7.0.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{DiscoveryZenHostsProvider})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{DiscoverySeedProviders})))
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
				v1beta1.HTTPConfig{},
				v1beta1.Config{Data: tt.cfgData},
				&certificates.CertificateResources{},
			)
			require.NoError(t, err)
			tt.assert(cfg)
		})
	}
}
