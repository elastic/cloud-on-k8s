// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
)

func TestRenderScriptTemplate(t *testing.T) {
	expectedAnnotations := "topology.kubernetes.io/zone topology.kubernetes.io/region"
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
						{
							Source: "/secrets/users",
							Target: "/usr/share/elasticsearch/users"}}},
			},
			wantSubstr: []string{
				"yes | cp -avf /usr/share/elasticsearch/config/* /mnt/elastic-internal/elasticsearch-config-local/",
				"yes | cp -avf /usr/share/elasticsearch/bin/* /mnt/elastic-internal/elasticsearch-bin-local/",
				"yes | cp -avf /usr/share/elasticsearch/plugins/* /mnt/elastic-internal/elasticsearch-plugins-local/",
				"ln -sf /secrets/users /usr/share/elasticsearch/users",
			},
			dontWantSubstr: []string{
				"expected_annotations",
			},
		},
		{
			name: "With expected annotations",
			params: TemplateParams{
				PluginVolumes:       PluginVolumes,
				ExpectedAnnotations: &expectedAnnotations,
			},
			wantSubstr: []string{
				"echo \"Waiting for the following annotations to be set on Pod: topology.kubernetes.io/zone topology.kubernetes.io/region\"",
				"expected_annotations=(topology.kubernetes.io/zone topology.kubernetes.io/region)",
				"while ! annotations_exist \"${expected_annotations[@]}\"; do sleep 2; done",
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

func TestRenderPrepareFsScript_Plugins(t *testing.T) {
	pluginsLink := "ln -sf /mnt/elastic-internal/elasticsearch-config/elasticsearch-plugins.yml /mnt/elastic-internal/elasticsearch-config-local/elasticsearch-plugins.yml"
	expectedAnnotations := []string{"topology.kubernetes.io/zone", "topology.kubernetes.io/region"}
	linkedFilesCount := len(linkedFiles.Array)
	tests := []struct {
		name                string
		expectedAnnotations []string
		withPlugins         bool
		wantSubstr          []string
		dontWantSubstr      []string
	}{
		{
			name:           "without plugins",
			withPlugins:    false,
			wantSubstr:     []string{"ln -sf /mnt/elastic-internal/elasticsearch-config/elasticsearch.yml"},
			dontWantSubstr: []string{pluginsLink},
		},
		{
			name:                "without plugins, with expected annotations",
			expectedAnnotations: expectedAnnotations,
			withPlugins:         false,
			wantSubstr:          []string{"ln -sf /mnt/elastic-internal/elasticsearch-config/elasticsearch.yml"},
			dontWantSubstr:      []string{pluginsLink},
		},
		{
			name:        "with plugins",
			withPlugins: true,
			wantSubstr:  []string{"ln -sf /mnt/elastic-internal/elasticsearch-config/elasticsearch.yml", pluginsLink},
		},
		{
			name:                "with plugins, with expected annotations",
			expectedAnnotations: expectedAnnotations,
			withPlugins:         true,
			wantSubstr:          []string{"ln -sf /mnt/elastic-internal/elasticsearch-config/elasticsearch.yml", pluginsLink},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := RenderPrepareFsScript(tt.expectedAnnotations, tt.withPlugins)
			assert.NoError(t, err)
			for _, substr := range tt.wantSubstr {
				assert.Contains(t, script, substr)
			}
			for _, substr := range tt.dontWantSubstr {
				assert.NotContains(t, script, substr)
			}
			// The script is part of the pods config hash: the snapshots of scripts without plugins must match the
			// script rendered before spec.plugins was introduced, otherwise every existing cluster rolls on operator upgrade.
			// Update with UPDATE_SNAPS=true after an intended script change.
			snaps.MatchSnapshot(t, script)
			// the shared linked files must not be mutated
			assert.Len(t, linkedFiles.Array, linkedFilesCount)
		})
	}
}
