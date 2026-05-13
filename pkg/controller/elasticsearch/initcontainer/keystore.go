// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

const (
	KeystoreBinPath = "/usr/share/elasticsearch/bin/elasticsearch-keystore"
)

// keystoreScript creates an Elasticsearch keystore and adds all secure
// settings in a single `elasticsearch-keystore add-file` invocation. Adding
// settings one at a time would incur the JVM startup cost for every entry,
// which can dominate pod startup when many secure settings are referenced
// (for example via StackConfigPolicy). Variadic add-file is supported by
// Elasticsearch since 7.7 (elastic/elasticsearch#54240).
const keystoreScript = `#!/usr/bin/env bash

set -eux

{{ if not .SkipInitializedFlag -}}
keystore_initialized_flag={{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok

if [[ -f "${keystore_initialized_flag}" ]]; then
    echo "Keystore already initialized."
	exit 0
fi

{{ end -}}
echo "Initializing keystore."

# create a keystore in the default data path
{{ .KeystoreCreateCommand }}

# collect all existing secret entries as (setting, path) pairs
add_args=()
for filename in {{ .SecureSettingsVolumeMountPath }}/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	add_args+=("$(basename "$filename")" "$filename")
done

# add all entries in a single keystore invocation to amortize JVM startup
if [[ ${#add_args[@]} -gt 0 ]]; then
	echo "Adding $(( ${#add_args[@]} / 2 )) entries to the keystore."
	{{ .KeystoreAddCommand }}
fi

{{ if not .SkipInitializedFlag -}}
touch {{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok
{{ end -}}

echo "Keystore initialization successful."
`

// KeystoreParams is used to generate the init container that will load the secure settings into a keystore.
var KeystoreParams = keystore.InitContainerParameters{
	KeystoreCreateCommand:         KeystoreBinPath + " create",
	KeystoreAddCommand:            KeystoreBinPath + ` add-file "${add_args[@]}"`,
	CustomScript:                  keystoreScript,
	SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
	KeystoreVolumePath:            esvolume.ConfigVolumeMountPath,
	Resources: corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("196Mi"),
			corev1.ResourceCPU:    resource.MustParse("500m"),
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("196Mi"),
			corev1.ResourceCPU:    resource.MustParse("500m"),
		},
	},
}
