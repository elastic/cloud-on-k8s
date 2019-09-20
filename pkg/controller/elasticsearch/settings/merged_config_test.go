// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates"
	"github.com/stretchr/testify/require"
)

func TestNewMergedESConfig(t *testing.T) {
	nodeML := "node.ml"
	xPackSecurityAuthcRealmsNativeNative1Order := "xpack.security.authc.realms.native.native1.order"
	xPackSecurityAuthcRealmsNative1Type := "xpack.security.authc.realms.native1.type"
	xPackSecurityAuthcRealmsNative1Order := "xpack.security.authc.realms.native1.order"

	tests := []struct {
		name    string
		version string
		cfgData map[string]interface{}
		assert  func(cfg CanonicalConfig)
	}{
		{
			name:    "in 6.x, empty config should have the default file realm settings configured",
			version: "6.8.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Order})))
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
			},
		},
		{
			name:    "in 6.x, native realm settings should be merged with the default file realm settings",
			version: "6.8.0",
			cfgData: map[string]interface{}{
				nodeML:                               true,
				xPackSecurityAuthcRealmsNative1Type:  "native",
				xPackSecurityAuthcRealmsNative1Order: 0,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsNative1Type})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsNative1Order})))
			},
		},
		{
			name:    "in 7.x, empty config should have the default file realm settings configured",
			version: "7.3.0",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
			},
		},
		{
			name:    "in 7.x, sample config should have the default file realm settings configured",
			version: "7.3.0",
			cfgData: map[string]interface{}{
				nodeML: true,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
			},
		},
		{
			name:    "in 7.x, native realm settings should be merged with the default file realm settings",
			version: "7.3.0",
			cfgData: map[string]interface{}{
				nodeML: true,
				xPackSecurityAuthcRealmsNativeNative1Order: 0,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{xPackSecurityAuthcRealmsNativeNative1Order})))
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
				v1alpha1.HTTPConfig{},
				v1alpha1.Config{Data: tt.cfgData},
				&certificates.CertificateResources{},
			)
			require.NoError(t, err)
			tt.assert(cfg)
		})
	}
}
