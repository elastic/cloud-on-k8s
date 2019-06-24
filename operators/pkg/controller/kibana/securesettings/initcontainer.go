// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	kbvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	InitContainerName = "init-keystore"
)

// script is a small bash script to create a Kibana keystore,
// then add all entries from the secure settings secret volume into it.
const script = `#!/usr/bin/env bash

set -eu

echo "Initializing Kibana keystore."

# create a keystore in the default data path
./bin/kibana-keystore create

# add all existing secret entries into it
for filename in ` + kbvolume.SecureSettingsVolumeMountPath + `/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	./bin/kibana-keystore add "$key" --stdin < "$filename"
done

echo "Keystore initialization successful."
`

// initContainer returns an init container that executes a bash script
// to create the Kibana Keystore.
func initContainer(secureSettingsSecret volume.SecretVolume) corev1.Container {
	privileged := false
	return corev1.Container{
		// Image will be inherited from pod template defaults Kibana Docker image
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", script},
		VolumeMounts: []corev1.VolumeMount{
			// access secure settings
			secureSettingsSecret.VolumeMount(),
			// write the keystore in Kibana data volume
			kbvolume.KibanaDataVolume.VolumeMount(),
		},
	}
}
