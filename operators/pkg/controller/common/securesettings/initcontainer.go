// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	InitContainerName = "init-keystore"
)

// initContainer returns an init container that executes a bash script
// to create the Keystore.
func initContainer(
	secureSettingsSecret volume.SecretVolume,
	secureSettingsVolumeMountPath string,
	dataVolumeMount corev1.VolumeMount,
	keystoreBinaryName string,
) corev1.Container {
	privileged := false
	return corev1.Container{
		// Image will be inherited from pod template defaults custom resource Docker image
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", script(secureSettingsVolumeMountPath, keystoreBinaryName)},
		VolumeMounts: []corev1.VolumeMount{
			// access secure settings
			secureSettingsSecret.VolumeMount(),
			// write the keystore in the custom resource data volume
			dataVolumeMount,
		},
	}
}

// script is a small bash script to create a keystore,
// then add all entries from the secure settings secret volume into it.
func script(secureSettingsVolumeMountPath string, keystoreBinaryName string) string {
	return `#!/usr/bin/env bash

set -eu

echo "Initializing keystore."

# create a keystore in the default data path
./bin/` + keystoreBinaryName + ` create

# add all existing secret entries into it
for filename in ` + secureSettingsVolumeMountPath + `/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	./bin/` + keystoreBinaryName + ` add "$key" --stdin < "$filename"
done

echo "Keystore initialization successful."
`
}
