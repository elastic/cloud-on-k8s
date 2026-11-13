// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorepassword

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"

// elasticsearchPasswordProtectedKeystoreScript creates a password-protected
// Elasticsearch keystore and adds all secure settings using batched
// `elasticsearch-keystore add-file` invocations. Adding settings one at a
// time would incur the JVM startup cost (and a password prompt) for every
// entry, which can dominate pod startup when many secure settings are
// referenced. Variadic add-file is supported by Elasticsearch since 7.7
// (elastic/elasticsearch#54240).
//
// Entries are chunked at 500 pairs per invocation to stay well below the
// kernel's ARG_MAX (~2 MiB total argv+envp) and MAX_ARG_STRLEN (128 KiB
// per single argument), see the matching note on initcontainer.keystoreScript.
//
// #nosec G101 -- this is a shell template variable name, not a hardcoded credential.
const elasticsearchPasswordProtectedKeystoreScript = `#!/usr/bin/env bash

set -eu

{{ if not .SkipInitializedFlag -}}
keystore_initialized_flag={{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok

if [[ -f "${keystore_initialized_flag}" ]]; then
    echo "Keystore already initialized."
	exit 0
fi

{{ end -}}
echo "Initializing keystore."

# Remove any existing keystore to avoid interactive "Overwrite?" prompt
rm -f {{ .KeystoreVolumePath }}/elasticsearch.keystore

KEYSTORE_PASSWORD=$(cat "{{ .KeystorePasswordPath }}")

# create a password-protected keystore; printf supplies password twice (new + confirmation)
printf "%s\n%s\n" "$KEYSTORE_PASSWORD" "$KEYSTORE_PASSWORD" | {{ .KeystoreCreateCommand }} -p

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
		echo -n "$KEYSTORE_PASSWORD" | {{ .KeystoreAddCommand }} "${add_args[@]:i:batch_pairs * 2}"
	done
fi
unset KEYSTORE_PASSWORD
{{ if not .SkipInitializedFlag -}}
touch {{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok
{{ end -}}

echo "Keystore initialization successful."
`

// ApplyPasswordProtectedKeystoreScript sets the Elasticsearch-specific custom
// keystore init script when a keystore password file is configured.
func ApplyPasswordProtectedKeystoreScript(parameters *keystore.InitContainerParameters) {
	if parameters == nil || parameters.KeystorePasswordPath == "" {
		return
	}
	parameters.CustomScript = elasticsearchPasswordProtectedKeystoreScript
}
