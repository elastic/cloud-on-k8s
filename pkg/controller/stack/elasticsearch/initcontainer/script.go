package initcontainer

import "html/template"

var pluginsToInstall = []string{
	"repository-s3",  // S3 snapshots
	"repository-gcs", // gcp snapshots
}

// TemplateParams are the parameters manipulated in the scriptTemplate
type TemplateParams struct {
	SetVMMaxMapCount bool              // Set vm.max_map_count=262144
	Plugins          []string          // List of plugins to install
	SharedVolumes    SharedVolumeArray // Volumes and directories that should be persisted
}

var scriptTemplate = template.Must(template.New("").Parse(
	`#!/usr/bin/env bash -eu

	ES_DIR="/usr/share/elasticsearch"
	PLUGIN_BIN=$ES_DIR/bin/elasticsearch-plugin

	# compute time in seconds since the given start time
	function duration() {
		local start=$1
		end=$(date +%s)
		echo $((end-start))
	}

	######################
	#        START       #
	######################

	script_start=$(date +%s)

	echo "Starting init script"

	######################
	#      OS Tweaks     #
	######################

	{{if .SetVMMaxMapCount}}
		# Set vm.max_map_count to a larger value as recommended in https://www.elastic.co/guide/en/elasticsearch/reference/current/docker.html
		echo "Setting vm.max_map_count"
		sysctl -w vm.max_map_count=262144
	{{end}}

	######################
	#       Plugins      #
	######################

	plugins_start=$(date +%s)
	# Install extra plugins
	{{range .Plugins}}
		echo "Installing plugin {{.}}"
		# Using --batch accepts any user prompt (y/n)
		$PLUGIN_BIN install --batch {{.}}
	{{end}}

	echo "Installed plugins:"
	$PLUGIN_BIN list

	echo "Plugins installation duration: $(duration $plugins_start) sec."

	######################
	#  Files persistence #
	######################

	# Persist the content of bin/, config/ and plugins/
	# to a volume, to be used by the ES container
	mv_start=$(date +%s)
	{{range .SharedVolumes.Array}}
		echo "Moving {{.EsContainerMountPath}}/* to {{.InitContainerMountPath}}/"
		mv {{.EsContainerMountPath}}/* {{.InitContainerMountPath}}/
	{{end}}
	echo "Files copy duration: $(duration $mv_start) sec."

	######################
	#         End        #
	######################

	echo "Init script successful"
	echo "Script duration: $(duration $script_start) sec."
`))
