// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update-golden", false, "update golden files")

func TestRenderPreStopHookScript(t *testing.T) {
	const svcURL = "https://test-es-http.default.svc:9200"

	tests := []struct {
		name                         string
		clientAuthenticationRequired bool
		goldenFile                   string
	}{
		{
			name:                         "without client authentication",
			clientAuthenticationRequired: false,
			goldenFile:                   "pre_stop_hook.golden",
		},
		{
			name:                         "with client authentication",
			clientAuthenticationRequired: true,
			goldenFile:                   "pre_stop_hook_client_auth.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderPreStopHookScript(svcURL, tt.clientAuthenticationRequired)
			require.NoError(t, err)

			goldenPath := filepath.Join("testdata", tt.goldenFile)

			if *updateGolden {
				require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0644))
				return
			}

			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err)
			require.Equal(t, string(want), got, "rendered script does not match golden file %s; run with -update-golden to update", tt.goldenFile)
		})
	}
}
