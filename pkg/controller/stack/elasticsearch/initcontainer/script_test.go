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
				SetVMMaxMapCount: true,
				Plugins:          defaultInstalledPlugins,
				SharedVolumes:    SharedVolumes,
			},
			wantSubstr: []string{
				"sysctl -w vm.max_map_count=262144",
				"$PLUGIN_BIN install --batch repository-s3",
				"$PLUGIN_BIN install --batch repository-gcs",
				"mv /usr/share/elasticsearch/config/* /volume/config/",
				"mv /usr/share/elasticsearch/bin/* /volume/bin/",
				"mv /usr/share/elasticsearch/plugins/* /volume/plugins/",
			},
		},
		{
			name: "No vm.max_map_count",
			params: TemplateParams{
				SetVMMaxMapCount: false,
				Plugins:          defaultInstalledPlugins,
				SharedVolumes:    SharedVolumes,
			},
			dontWantSubstr: []string{
				"sysctl -w vm.max_map_count=262144",
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
