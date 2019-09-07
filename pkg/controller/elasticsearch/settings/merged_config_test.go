// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestNewMergedESConfig(t *testing.T) {
	nodeML := "node.ml"
	xPackSecurityAuthcRealmsNativeNative1Order := "xpack.security.authc.realms.native.native1.order"

	tests := []struct {
		name    string
		cfgData map[string]interface{}
		assert  func(cfg CanonicalConfig)
	}{
		{
			name:    "empty config should have the default file realm settings configured",
			cfgData: map[string]interface{}{},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 0, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
			},
		},
		{
			name: "sample config should have the default file realm settings configured",
			cfgData: map[string]interface{}{
				nodeML: true,
			},
			assert: func(cfg CanonicalConfig) {
				require.Equal(t, 1, len(cfg.HasKeys([]string{nodeML})))
				require.Equal(t, 1, len(cfg.HasKeys([]string{XPackSecurityAuthcRealmsFileFile1Order})))
			},
		},
		{
			name: "native realm settings should be merged with the default file realm settings",
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
			cfg, err := NewMergedESConfig("clusterName", v1alpha1.HTTPConfig{}, v1alpha1.Config{
				Data: tt.cfgData,
			})
			require.NoError(t, err)
			tt.assert(cfg)
		})
	}
}
