// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderScriptTemplate(t *testing.T) {
	tests := []struct {
		name           string
		params         TemplateParams
		wantSubstr     []string
		dontWantSubstr []string
	}{
		{
			name: "Standard script rendering",
			params: TemplateParams{
				Plugins:       defaultInstalledPlugins,
				SharedVolumes: PrepareFsSharedVolumes,
				LinkedFiles: LinkedFilesArray{
					Array: []LinkedFile{
						LinkedFile{
							Source: "/secrets/users",
							Target: "/usr/share/elasticsearch/users"}}},
			},
			wantSubstr: []string{
				// TODO: re-enable when these are used
				// "$PLUGIN_BIN install --batch repository-s3",
				// "$PLUGIN_BIN install --batch repository-gcs",
				"mv /usr/share/elasticsearch/config/* /volume/config/",
				"mv /usr/share/elasticsearch/bin/* /volume/bin/",
				"mv /usr/share/elasticsearch/plugins/* /volume/plugins/",
				"ln -sf /secrets/users /usr/share/elasticsearch/users",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := RenderScriptTemplate(tt.params)
			assert.NoError(t, err)

			for _, substr := range tt.wantSubstr {
				assert.Contains(t, script, substr)
			}
			for _, substr := range tt.dontWantSubstr {
				assert.NotContains(t, script, substr)
			}
		})
	}
}
