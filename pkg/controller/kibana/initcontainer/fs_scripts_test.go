// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRenderScriptTemplate(t *testing.T) {
	type args struct {
		params templateParams
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "template renders without plugin section",
			args: args{params: templateParams{
				ContainerPluginsMountPath:     "/usr/share/kibana/plugins",
				InitContainerPluginsMountPath: "/mnt/elastic-internal/kibana-plugins-local",
				IncludePlugins:                false,
			}},
			want: `#!/usr/bin/env bash
set -eux



init_config_initialized_flag=/mnt/elastic-internal/kibana-config-local/elastic-internal-init-config.ok

if [[ -f "${init_config_initialized_flag}" ]]; then
	echo "Kibana configuration already initialized."
	exit 0
fi

echo "Setup Kibana configuration"

ln -sf /mnt/elastic-internal/kibana-config/* /mnt/elastic-internal/kibana-config-local/

touch "${init_config_initialized_flag}"
echo "Kibana configuration successfully prepared."
`,
			wantErr: false,
		},
		{
			name: "template renders with plugin section",
			args: args{params: templateParams{
				ContainerPluginsMountPath:     "/usr/share/kibana/plugins",
				InitContainerPluginsMountPath: "/mnt/elastic-internal/kibana-plugins-local",
				IncludePlugins:                true,
			}},
			want: `#!/usr/bin/env bash
set -eux



# compute time in seconds since the given start time
function duration() {
	local start=$1
	end=$(date +%s)
	echo $((end-start))
}

#######################
# Plugins persistence #
#######################

# Persist the content of plugins/ to a volume, so installed
# plugins files can to be used by the Kibana container.
mv_start=$(date +%s)
if [[ -z "$(ls -A /usr/share/kibana/plugins)" ]]; then
	echo "Empty dir /usr/share/kibana/plugins"
else
	echo "Copying /usr/share/kibana/plugins/* to /mnt/elastic-internal/kibana-plugins-local/"
	# Use "yes" and "-f" as we want the init container to be idempotent and not to fail when executed more than once.
	yes | cp -avf /usr/share/kibana/plugins/* /mnt/elastic-internal/kibana-plugins-local/
fi
echo "Files copy duration: $(duration $mv_start) sec."



init_config_initialized_flag=/mnt/elastic-internal/kibana-config-local/elastic-internal-init-config.ok

if [[ -f "${init_config_initialized_flag}" ]]; then
	echo "Kibana configuration already initialized."
	exit 0
fi

echo "Setup Kibana configuration"

ln -sf /mnt/elastic-internal/kibana-config/* /mnt/elastic-internal/kibana-config-local/

touch "${init_config_initialized_flag}"
echo "Kibana configuration successfully prepared."
`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderScriptTemplate(tt.args.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderScriptTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RenderScriptTemplate() diff = %s", cmp.Diff(got, tt.want))
			}
		})
	}
}
