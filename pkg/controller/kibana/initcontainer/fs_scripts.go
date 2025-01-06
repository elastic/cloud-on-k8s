// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"bytes"
	"html/template"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/settings"
)

const (
	KibanaInitScriptConfigKey = "init.sh"
)

// TemplateParams are the parameters manipulated in the pluginsFsScriptTemplate
type TemplateParams struct {
	// ContainerPluginsMountPath is the mount path for plugins
	// within the Kibana container.
	ContainerPluginsMountPath string
	// InitContainerPluginsMountPath is the mount path for plugins
	// within the init container.
	InitContainerPluginsMountPath string
	// IncludePlugins indicates whether the script should include
	// the plugins persistence logic.
	IncludePlugins bool
}

var initFsScriptTemplate = template.Must(template.New("").Parse(
	`#!/usr/bin/env bash
	set -eux

	init_config_initialized_flag=` + settings.InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok

	if [[ -f "${init_config_initialized_flag}" ]]; then
		echo "Kibana configuration already initialized."
		exit 0
	fi

	echo "Setup Kibana configuration"

	ln -sf ` + settings.InternalConfigVolumeMountPath + `/* ` + settings.InitContainerConfigVolumeMountPath + `/

	touch "${init_config_initialized_flag}"
	echo "Kibana configuration successfully prepared."

	{{ if .IncludePlugins }}

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
	if [[ -z "$(ls -A {{.ContainerPluginsMountPath}})" ]]; then
		echo "Empty dir {{.ContainerPluginsMountPath}}"
	else
		echo "Copying {{.ContainerPluginsMountPath}}/* to {{.InitContainerPluginsMountPath}}/"
		# Use "yes" and "-f" as we want the init container to be idempotent and not to fail when executed more than once.
		yes | cp -avf {{.ContainerPluginsMountPath}}/* {{.InitContainerPluginsMountPath}}/ 
	fi
	echo "Files copy duration: $(duration $mv_start) sec."

	{{ end }}
`))

// RenderScriptTemplate renders initFsScriptTemplate using the given TemplateParams
func RenderScriptTemplate(params TemplateParams) (string, error) {
	tplBuffer := bytes.Buffer{}
	if err := initFsScriptTemplate.Execute(&tplBuffer, params); err != nil {
		return "", err
	}
	return tplBuffer.String(), nil
}
