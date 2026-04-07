// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorepassword

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"

// #nosec G101 -- this is a shell template variable name, not a hardcoded credential.
const elasticsearchPasswordProtectedKeystoreScript = `#!/usr/bin/env bash

set -eux

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

# add all existing secret entries into it
for filename in  {{ .SecureSettingsVolumeMountPath }}/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	echo -n "$KEYSTORE_PASSWORD" | {{ .KeystoreAddCommand }}
done
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
