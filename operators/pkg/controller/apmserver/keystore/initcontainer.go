// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"bytes"
	"text/template"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	InitContainerName = "init-keystore"
)

type InitContainerParameters struct {
	// Where the user provided secured settings should be mounted
	SecureSettingsVolumeMountPath string
	// Keystore command
	KeystoreCommand string
}

// script is a small bash script to create a Kibana or APM keystore,
// then add all entries from the secure settings secret volume into it.
const script = `#!/usr/bin/env bash

set -eux

echo "Initializing keystore."

# create a keystore in the default data path
{{ .KeystoreCommand }} create

# add all existing secret entries into it
for filename in  {{ .SecureSettingsVolumeMountPath }}/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	{{ .KeystoreCommand }} add "$key" --stdin < "$filename"
done

echo "Keystore initialization successful."
`

var scriptTemplate = template.Must(template.New("").Parse(script))

// initContainer returns an init container that executes a bash script
// to create the APM Keystore.
func initContainer(
	object runtime.Object,
	secureSettingsSecret volume.SecretVolume,
	dataVolumePath string,
	KeystoreCommand string,
) (corev1.Container, error) {
	privileged := false
	params := InitContainerParameters{
		SecureSettingsVolumeMountPath: SecureSettingsVolumeMountPath,
		KeystoreCommand:               KeystoreCommand,
	}
	tplBuffer := bytes.Buffer{}
	if err := scriptTemplate.Execute(&tplBuffer, params); err != nil {
		return corev1.Container{}, err
	}

	return corev1.Container{
		// Image will be inherited from pod template defaults Kibana Docker image
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", tplBuffer.String()},
		VolumeMounts: []corev1.VolumeMount{
			// access secure settings
			secureSettingsSecret.VolumeMount(),
			// write the keystore in the data volume
			DataVolume(object, dataVolumePath).VolumeMount(),
		},
	}, nil
}
