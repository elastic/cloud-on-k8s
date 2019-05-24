// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"bytes"
	"html/template"
)

// List of plugins to be installed on the ES instance
var defaultInstalledPlugins = []string{
	"repository-s3",  // S3 snapshots
	"repository-gcs", // gcp snapshots
}

// TemplateParams are the parameters manipulated in the scriptTemplate
type TemplateParams struct {
	// Plugins is a list of plugins to install
	Plugins []string
	// SharedVolumes are directories to persist in shared volumes
	SharedVolumes SharedVolumeArray
	// LinkedFiles are files to link individually
	LinkedFiles LinkedFilesArray
	// ChownToElasticsearch are paths that need to be chowned to the Elasticsearch user/group.
	ChownToElasticsearch []string
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
	CONFIG_DIR=$ES_DIR/config
	PLUGIN_BIN=$ES_DIR/bin/elasticsearch-plugin
	KEYSTORE_BIN=$ES_DIR/bin/elasticsearch-keystore 

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
		if [[ -z "$(ls -A {{.EsContainerMountPath}})" ]]; then
			echo "Empty dir {{.EsContainerMountPath}}"
		else
			echo "Moving {{.EsContainerMountPath}}/* to {{.InitContainerMountPath}}/"
			mv {{.EsContainerMountPath}}/* {{.InitContainerMountPath}}/
		fi
	{{end}}
	echo "Files copy duration: $(duration $mv_start) sec."

	######################
	#  Volumes chown     #
	######################

	# chown the data and logs volume to the elasticsearch user
	chown_start=$(date +%s)
	{{range .ChownToElasticsearch}}
		echo "chowning {{.}} to elasticsearch:elasticsearch"
		chown -v elasticsearch:elasticsearch {{.}}
	{{end}}
	echo "chown duration: $(duration $chown_start) sec."

	######################
	#         End        #
	######################

	echo "Init script successful"
	echo "Script duration: $(duration $script_start) sec."
`))
