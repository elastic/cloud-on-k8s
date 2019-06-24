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
				PluginVolumes: PluginVolumes,
				LinkedFiles: LinkedFilesArray{
					Array: []LinkedFile{
						LinkedFile{
							Source: "/secrets/users",
							Target: "/usr/share/elasticsearch/users"}}},
			},
			wantSubstr: []string{
				"cp -av /usr/share/elasticsearch/config/* /mnt/elastic-internal/elasticsearch-config-local/",
				"cp -av /usr/share/elasticsearch/bin/* /mnt/elastic-internal/elasticsearch-bin-local/",
				"cp -av /usr/share/elasticsearch/plugins/* /mnt/elastic-internal/elasticsearch-plugins-local/",
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
