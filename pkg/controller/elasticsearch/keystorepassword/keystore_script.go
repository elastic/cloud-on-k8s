// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorepassword

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"

// elasticsearchPasswordProtectedKeystoreScript creates a password-protected
// Elasticsearch keystore and adds all secure settings in a single
// `elasticsearch-keystore add-file` invocation. Adding settings one at a
// time would incur the JVM startup cost (and a password prompt) for every
// entry, which can dominate pod startup when many secure settings are
// referenced. Variadic add-file is supported by Elasticsearch since 7.7
// (elastic/elasticsearch#54240).
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

# add all entries in a single keystore invocation to amortize JVM startup
if [[ ${#add_args[@]} -gt 0 ]]; then
	echo "Adding $(( ${#add_args[@]} / 2 )) entries to the keystore."
	echo -n "$KEYSTORE_PASSWORD" | {{ .KeystoreAddCommand }}
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
