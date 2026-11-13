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
// settings using batched `elasticsearch-keystore add-file` invocations.
// Adding settings one at a time would incur the JVM startup cost for every
// entry, which can dominate pod startup when many secure settings are
// referenced (for example via StackConfigPolicy). Variadic add-file is
// supported by Elasticsearch since 7.7 (elastic/elasticsearch#54240), which
// is below the minimum Elasticsearch version ECK supports once #9041 lands.
//
// Entries are chunked at 500 pairs per invocation. Worst-case per pair is
// ~540 B (253-char setting name + ~290-char path), so a chunk is at most
// ~270 KiB of argv, well below the kernel's ARG_MAX (~2 MiB) and far below
// MAX_ARG_STRLEN (128 KiB per single argument). Realistic per-pair sizes
// are ~150 B, so the customer-reported 50–100 secret workload still runs
// in a single keystore invocation.
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

# Add entries in chunks to amortize JVM startup while staying well below
# the kernel's ARG_MAX / MAX_ARG_STRLEN limits even with very large fleets.
#
# add_args interleaves (setting, path) for each keystore entry, so:
#   ${#add_args[@]}      = total array slots = 2 * number of entries
#   ${#add_args[@]} / 2  = number of entries (human-meaningful count)
#   batch_pairs * 2      = array slots per chunk (we want batch_pairs entries)
# The loop strides through add_args two slots at a time per entry, slicing
# batch_pairs entries into argv per invocation.
batch_pairs=500
if [[ ${#add_args[@]} -gt 0 ]]; then
	echo "Adding $(( ${#add_args[@]} / 2 )) entries to the keystore."
	for (( i = 0; i < ${#add_args[@]}; i += batch_pairs * 2 )); do
		{{ .KeystoreAddCommand }} "${add_args[@]:i:batch_pairs * 2}"
	done
fi

{{ if not .SkipInitializedFlag -}}
touch {{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok
{{ end -}}

echo "Keystore initialization successful."
`

// KeystoreParams is used to generate the init container that will load the secure settings into a keystore.
var KeystoreParams = keystore.InitContainerParameters{
	KeystoreCreateCommand: KeystoreBinPath + " create",
	// KeystoreAddCommand is the bare `add-file` invocation; the keystoreScript
	// template appends a chunked slice of "${add_args[@]:i:batch_pairs * 2}"
	// at each call site so we can batch entries while staying under the
	// kernel argv limits.
	KeystoreAddCommand:            KeystoreBinPath + " add-file",
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
