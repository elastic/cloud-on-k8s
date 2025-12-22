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
			name: "template renders with plugin section",
			args: args{params: templateParams{
				ContainerPluginsMountPath:     "/usr/share/kibana/plugins",
				InitContainerPluginsMountPath: "/mnt/elastic-internal/kibana-plugins-local",
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

init_plugins_copied_flag=/mnt/elastic-internal/kibana-plugins-local/elastic-internal-init-plugins.ok

# Persist the content of plugins/ to a volume, so installed
# plugins files can to be used by the Kibana container.
mv_start=$(date +%s)
if [[ ! -f "${init_plugins_copied_flag}" ]]; then
	if [[ -z "$(ls -A /usr/share/kibana/plugins)" ]]; then
		echo "Empty dir /usr/share/kibana/plugins"
	else
		echo "Copying /usr/share/kibana/plugins/* to /mnt/elastic-internal/kibana-plugins-local/"
		# Use "yes" and "-f" as we want the init container to be idempotent and not to fail when executed more than once.
		yes | cp -avf /usr/share/kibana/plugins/* /mnt/elastic-internal/kibana-plugins-local/
	fi
fi
touch "${init_plugins_copied_flag}"
echo "Files copy duration: $(duration $mv_start) sec."

#############################
# CA Certificate Management #
#############################

# Handle EPR CA certificate consolidation with user-provided NODE_EXTRA_CA_CERTS
# This must run before the early exit to ensure certificates are always processed
if [[ -f "/usr/share/kibana/config/epr-certs/ca.crt" ]]; then
	echo "EPR CA certificate found, checking for user-provided CA bundle..."
	
	# Check if user provided their own NODE_EXTRA_CA_CERTS (from init container env)
	if [[ -n "${NODE_EXTRA_CA_CERTS:-}" ]]; then
		echo "User provided NODE_EXTRA_CA_CERTS: $NODE_EXTRA_CA_CERTS"
		if [[ -f "$NODE_EXTRA_CA_CERTS" ]]; then
			# Create combined CA bundle in config directory for main container
			COMBINED_CA_PATH="/mnt/elastic-internal/kibana-config-local/combined-ca-bundle.crt"
			echo "Creating combined CA bundle at: $COMBINED_CA_PATH"
			cp "$NODE_EXTRA_CA_CERTS" "$COMBINED_CA_PATH"
			cat "/usr/share/kibana/config/epr-certs/ca.crt" >> "$COMBINED_CA_PATH"
			echo "Combined CA bundle created with user CA + EPR CA certificates."
		else
			echo "User-specified NODE_EXTRA_CA_CERTS file not found: $NODE_EXTRA_CA_CERTS"
			echo "Creating EPR-only CA bundle..."
			cp "/usr/share/kibana/config/epr-certs/ca.crt" "/mnt/elastic-internal/kibana-config-local/combined-ca-bundle.crt"
		fi
	else
		echo "No user CA bundle provided, EPR CA will be used directly via NODE_EXTRA_CA_CERTS."
	fi
fi

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
