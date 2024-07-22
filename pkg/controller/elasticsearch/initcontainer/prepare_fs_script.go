// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
)

// TemplateParams are the parameters manipulated in the scriptTemplate
type TemplateParams struct {
	// SharedVolumes are directories to persist in shared volumes
	PluginVolumes volume.SharedVolumeArray
	// LinkedFiles are files to link individually
	LinkedFiles LinkedFilesArray
	// ChownToElasticsearch are paths that need to be chowned to the Elasticsearch user/group.
	ChownToElasticsearch []string

	// ExpectedAnnotations are the annotations expected on the Pod. Init script waits until these annotations are set by
	// the operator.
	ExpectedAnnotations *string

	// InitContainerTransportCertificatesSecretVolumeMountPath is the path to the volume in the init container that
	// contains the transport certificates.
	InitContainerTransportCertificatesSecretVolumeMountPath string

	// TransportCertificatesSecretVolumeMountPath is the path to the volume in the es container that contains the
	// transport certificates.
	TransportCertificatesSecretVolumeMountPath string
}

// RenderScriptTemplate renders scriptTemplate using the given TemplateParams
func RenderScriptTemplate(params TemplateParams) (string, error) {
	tplBuffer := bytes.Buffer{}
	if err := scriptTemplate.Execute(&tplBuffer, params); err != nil {
		return "", err
	}
	return tplBuffer.String(), nil
}

const (
	PrepareFsScriptConfigKey  = "prepare-fs.sh"
	UnsupportedDistroExitCode = 42
)

// scriptTemplate is the main script to be run
// in the prepare-fs init container before ES starts
var scriptTemplate = template.Must(template.New("").Parse(
	`#!/usr/bin/env bash

	set -eu
{{ if .ExpectedAnnotations }}
	function annotations_exist() {
	  expected_annotations=("$@")
	  for expected_annotation in "${expected_annotations[@]}"; do
		annotation_exists=$(grep -c "^${expected_annotation}=" /mnt/elastic-internal/downward-api/annotations)
		if [ "${annotation_exists}" -eq 0 ]; then
			return 1
		fi
	  done
	  return 0
	}
{{ end }}
	# the operator only works with the default ES distribution
	license=/usr/share/elasticsearch/LICENSE.txt
	if [[ ! -f $license || $(grep -Exc "ELASTIC LICENSE AGREEMENT|Elastic License 2.0" $license) -ne 1 ]]; then
		>&2 echo "unsupported_distribution"
		exit ` + fmt.Sprintf("%d", UnsupportedDistroExitCode) + `
	fi

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
	#  Files persistence #
	######################

	# Persist the content of bin/, config/ and plugins/ to a volume,
	# so installed plugins files can to be used by the ES container
	mv_start=$(date +%s)
	{{range .PluginVolumes.Array}}
		if [[ -z "$(ls -A {{.ContainerMountPath}})" ]]; then
			echo "Empty dir {{.ContainerMountPath}}"
		else
			echo "Copying {{.ContainerMountPath}}/* to {{.InitContainerMountPath}}/"
			# Use "yes" and "-f" as we want the init container to be idempotent and not to fail when executed more than once.
			yes | cp -avf {{.ContainerMountPath}}/* {{.InitContainerMountPath}}/ 
		fi
	{{end}}
	echo "Files copy duration: $(duration $mv_start) sec."

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
	#  Volumes chown     #
	######################

	# chown the data and logs volume to the elasticsearch user
	# only done when running as root, other cases should be handled
	# with a proper security context
	chown_start=$(date +%s)
	if [[ $EUID -eq 0 ]]; then
		{{range .ChownToElasticsearch}}
			echo "chowning {{.}} to elasticsearch:elasticsearch"
			chown -v elasticsearch:elasticsearch {{.}}
		{{end}}
	fi
	echo "chown duration: $(duration $chown_start) sec."

	######################
	#  Wait for certs    #
	######################

	INIT_CONTAINER_LOCAL_KEY_PATH={{ .InitContainerTransportCertificatesSecretVolumeMountPath }}/${POD_NAME}.tls.key
	DISABLED_CERT_MARKER={{ .InitContainerTransportCertificatesSecretVolumeMountPath }}/transport.certs.disabled
	# wait for the transport certificates to show up
	echo "waiting for the transport certificates (${INIT_CONTAINER_LOCAL_KEY_PATH} or ${DISABLED_CERT_MARKER})"
	wait_start=$(date +%s)
	while [ ! -f ${INIT_CONTAINER_LOCAL_KEY_PATH} ] && [ ! -f ${DISABLED_CERT_MARKER} ]
	do
		sleep 0.2
	done
	echo "wait duration: $(duration wait_start) sec."
	if [ -f ${DISABLED_CERT_MARKER} ]; then
		echo "Skipped transport certificate check because of .spec.transport.tls.selfSignedCerts.disabled"
	fi

{{ if .ExpectedAnnotations }}
	echo "Waiting for the following annotations to be set on Pod: {{ .ExpectedAnnotations }}"
	ln_start=$(date +%s)
	declare -a expected_annotations
    expected_annotations=({{ .ExpectedAnnotations }})
  	while ! annotations_exist "${expected_annotations[@]}"; do sleep 2; done
	echo "Waiting for annotations duration: $(duration $ln_start) sec."
{{ end }}
	######################
	#         End        #
	######################
	echo "Init script successful"
	echo "Script duration: $(duration $script_start) sec."

`))
