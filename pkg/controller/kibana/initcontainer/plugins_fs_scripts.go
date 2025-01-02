// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"bytes"
	"html/template"
)

const (
	PrepareFsScriptConfigKey = "prepare-fs.sh"
)

// TemplateParams are the parameters manipulated in the pluginsFsScriptTemplate
type TemplateParams struct {
	// ContainerPluginsMountPath is the mount path for plugins
	// within the Kibana container.
	ContainerPluginsMountPath string
	// InitContainerPluginsMountPath is the mount path for plugins
	// within the init container.
	InitContainerPluginsMountPath string
}

var pluginsFsScriptTemplate = template.Must(template.New("").Parse(
	`#!/usr/bin/env bash

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
	{{end}}	echo "Files copy duration: $(duration $mv_start) sec."
`))

// RenderScriptTemplate renders pluginsFsScriptTemplate using the given TemplateParams
func RenderScriptTemplate(params TemplateParams) (string, error) {
	tplBuffer := bytes.Buffer{}
	if err := pluginsFsScriptTemplate.Execute(&tplBuffer, params); err != nil {
		return "", err
	}
	return tplBuffer.String(), nil
}
