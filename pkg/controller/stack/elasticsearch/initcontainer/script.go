package initcontainer

import (
	"bytes"
	"html/template"
)

// List of plugins to be installed on the ES instance
var defaultInstalledPlugins = []string{
	// TODO: enable when useful :)
	// "repository-s3",  // S3 snapshots
	// "repository-gcs", // gcp snapshots
}

// TemplateParams are the parameters manipulated in the scriptTemplate
type TemplateParams struct {
	Plugins       []string          // List of plugins to install
	SharedVolumes SharedVolumeArray // Directories to persist in shared volumes
	LinkedFiles   LinkedFilesArray  // Files to link individually
}

// RenderScriptTemplate renders scriptTemplate using the given TemplateParams
func RenderScriptTemplate(params TemplateParams) (string, error) {
	tplBuffer := bytes.Buffer{}
	if err := scriptTemplate.Execute(&tplBuffer, params); err != nil {
		return "", err
	}
	return tplBuffer.String(), nil
}

// scriptTemplate is the main script to be run
// in the prepare-fs init container before ES starts
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
	#  Config linking    #
	######################

	# Link individual files from their mount location into the config dir
	# to a volume, to be used by the ES container
	ln_start=$(date +%s)
	{{range .LinkedFiles.Array}}
		echo "Linking {{.Source}} to {{.Target}}"
		ln -sf {{.Source}} {{.Target}}
	{{end}}
	echo "File linking duration: $(duration $ln_start) sec."


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
