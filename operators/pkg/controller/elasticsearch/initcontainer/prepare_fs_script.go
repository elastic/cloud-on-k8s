// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"bytes"
	"html/template"
)

// TemplateParams are the parameters manipulated in the scriptTemplate
type TemplateParams struct {
	// SharedVolumes are directories to persist in shared volumes
	PluginVolumes SharedVolumeArray
	// LinkedFiles are files to link individually
	LinkedFiles LinkedFilesArray
	// ChownToElasticsearch are paths that need to be chowned to the Elasticsearch user/group.
	ChownToElasticsearch []string

	// InitContainerTransportCertificatesSecretVolumeMountPath is the path to the volume in the init container that
	// contains the transport certificates.
	InitContainerTransportCertificatesSecretVolumeMountPath string

	// InitContainerNodeTransportCertificatesKeyPath is the path within the init container where the private key for the
	// node transport certificates should be found.
	InitContainerNodeTransportCertificatesKeyPath string
	// InitContainerNodeTransportCertificatesCertPath is the path within the init container where the certificate for
	// the node transport should be found.
	InitContainerNodeTransportCertificatesCertPath string

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

const PrepareFsScriptConfigKey = "prepare-fs.sh"

// scriptTemplate is the main script to be run
// in the prepare-fs init container before ES starts
var scriptTemplate = template.Must(template.New("").Parse(
	`#!/usr/bin/env bash

	set -eu

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

	# Persist the content of bin/, config/ and plugins/ to a volume,
	# so installed plugins files can to be used by the ES container
	mv_start=$(date +%s)
	{{range .PluginVolumes.Array}}
		if [[ -z "$(ls -A {{.EsContainerMountPath}})" ]]; then
			echo "Empty dir {{.EsContainerMountPath}}"
		else
			echo "Copying {{.EsContainerMountPath}}/* to {{.InitContainerMountPath}}/"
			cp -av {{.EsContainerMountPath}}/* {{.InitContainerMountPath}}/
		fi
	{{end}}
	echo "Files copy duration: $(duration $mv_start) sec."

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

	# wait for the transport certificates to show up
	echo "waiting for the transport certificates (${INIT_CONTAINER_LOCAL_KEY_PATH})"
	wait_start=$(date +%s)
	while [ ! -f ${INIT_CONTAINER_LOCAL_KEY_PATH} ]
	do
	  sleep 0.2
	done
	echo "wait duration: $(duration wait_start) sec."

	######################
	#  Certs linking     #
	######################

	KEY_SOURCE_PATH={{ .TransportCertificatesSecretVolumeMountPath }}/${POD_NAME}.tls.key
	KEY_TARGET_PATH={{ .InitContainerNodeTransportCertificatesKeyPath }}

	CERT_SOURCE_PATH={{ .TransportCertificatesSecretVolumeMountPath }}/${POD_NAME}.tls.crt
	CERT_TARGET_PATH={{ .InitContainerNodeTransportCertificatesCertPath }}

	# Link individual files from their mount location into the config dir
	# to a volume, to be used by the ES container
	ln_start=$(date +%s)

	echo "Linking $CERT_SOURCE_PATH to $CERT_TARGET_PATH"
	mkdir -p $(dirname $KEY_TARGET_PATH)
	ln -sf $KEY_SOURCE_PATH $KEY_TARGET_PATH
	echo "Linking $CERT_SOURCE_PATH to $CERT_TARGET_PATH"
	mkdir -p $(dirname $CERT_TARGET_PATH)
	ln -sf $CERT_SOURCE_PATH $CERT_TARGET_PATH

	echo "Certs linking duration: $(duration $ln_start) sec."

	######################
	#         End        #
	######################

	echo "Init script successful"
	echo "Script duration: $(duration $script_start) sec."
`))
