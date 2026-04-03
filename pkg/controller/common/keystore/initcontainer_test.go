// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func renderKeystoreScript(t *testing.T, params InitContainerParameters) string {
	t.Helper()
	var tplBuffer bytes.Buffer
	err := getScriptTemplate(params.CustomScript).Execute(&tplBuffer, params)
	require.NoError(t, err)
	return tplBuffer.String()
}

func TestDefaultScriptByteIdentity(t *testing.T) {
	params := InitContainerParameters{
		KeystoreCreateCommand:         "/keystore/bin/keystore create",
		KeystoreAddCommand:            `/keystore/bin/keystore add "$key" "$filename"`,
		SecureSettingsVolumeMountPath: "/foo/secret",
		KeystoreVolumePath:            "/bar/data",
	}

	const expected = `#!/usr/bin/env bash

set -eux

keystore_initialized_flag=/bar/data/elastic-internal-init-keystore.ok

if [[ -f "${keystore_initialized_flag}" ]]; then
    echo "Keystore already initialized."
	exit 0
fi

echo "Initializing keystore."

# create a keystore in the default data path
/keystore/bin/keystore create

# add all existing secret entries into it
for filename in  /foo/secret/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	/keystore/bin/keystore add "$key" "$filename"
done

touch /bar/data/elastic-internal-init-keystore.ok
echo "Keystore initialization successful."
`

	require.Equal(t, expected, renderKeystoreScript(t, params))
}

func TestFIPSEnabledScriptRendering(t *testing.T) {
	params := InitContainerParameters{
		KeystoreCreateCommand:         "/keystore/bin/keystore create",
		KeystoreAddCommand:            `/keystore/bin/keystore add "$key" "$filename"`,
		SecureSettingsVolumeMountPath: "/foo/secret",
		KeystoreVolumePath:            "/bar/data",
		FIPSKeystorePasswordPath:      "/mnt/elastic-internal/fips-keystore-password/keystore-password",
	}

	rendered := renderKeystoreScript(t, params)
	require.Contains(t, rendered, "rm -f /bar/data/elasticsearch.keystore")
	require.Contains(t, rendered, `KEYSTORE_PASSWORD=$(cat "/mnt/elastic-internal/fips-keystore-password/keystore-password")`)
	require.Contains(t, rendered, `printf "%s\n%s\n" "$KEYSTORE_PASSWORD" "$KEYSTORE_PASSWORD" | /keystore/bin/keystore create -p`)
	require.Contains(t, rendered, `echo -n "$KEYSTORE_PASSWORD" | /keystore/bin/keystore add "$key" "$filename"`)
}

func TestFIPSEnabledScriptRenderingSkipInitializedFlag(t *testing.T) {
	params := InitContainerParameters{
		KeystoreCreateCommand:         "/keystore/bin/keystore create",
		KeystoreAddCommand:            `/keystore/bin/keystore add "$key" "$filename"`,
		SecureSettingsVolumeMountPath: "/foo/secret",
		KeystoreVolumePath:            "/bar/data",
		FIPSKeystorePasswordPath:      "/mnt/elastic-internal/fips-keystore-password/keystore-password",
		SkipInitializedFlag:           true,
	}

	rendered := renderKeystoreScript(t, params)
	require.False(t, strings.Contains(rendered, "keystore_initialized_flag="))
	require.Contains(t, rendered, "rm -f /bar/data/elasticsearch.keystore")
	require.Contains(t, rendered, `printf "%s\n%s\n" "$KEYSTORE_PASSWORD" "$KEYSTORE_PASSWORD" | /keystore/bin/keystore create -p`)
}
