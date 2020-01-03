// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/stretchr/testify/require"
)

func TestVersionDefaults(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		want    *settings.CanonicalConfig
	}{
		{
			name:    "6.x",
			version: "6.8.5",
			want:    settings.NewCanonicalConfig(),
		},
		{
			name:    "7.x",
			version: "7.1.0",
			want:    settings.NewCanonicalConfig(),
		},
		{
			name:    "7.6.0",
			version: "7.6.0",
			want: settings.MustCanonicalConfig(map[string]interface{}{
				XpackLicenseManagementUIEnabled: false,
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kb := &kbv1.Kibana{Spec: kbv1.KibanaSpec{Version: tc.version}}
			v := version.MustParse(tc.version)

			defaults := VersionDefaults(kb, v)
			var have map[string]interface{}
			require.NoError(t, defaults.Unpack(&have))

			var want map[string]interface{}
			require.NoError(t, tc.want.Unpack(&want))

			require.Equal(t, want, have)
		})
	}
}
