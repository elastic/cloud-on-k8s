// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
)

func TestKeystoreScript_BatchesAddFile(t *testing.T) {
	tpl, err := template.New("").Parse(KeystoreParams.CustomScript)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, tpl.Execute(&out, KeystoreParams))
	rendered := out.String()

	// Secrets are collected as (setting, path) pairs and added in a single
	// keystore invocation to amortize JVM startup cost.
	require.Contains(t, rendered, `add_args+=("$(basename "$filename")" "$filename")`)
	require.Contains(t, rendered, `/usr/share/elasticsearch/bin/elasticsearch-keystore add-file "${add_args[@]}"`)

	// The legacy per-file `add-file "$key" "$filename"` form must not appear
	// (each invocation incurred a full JVM startup, see elastic/cloud-on-k8s#9439).
	require.NotContains(t, rendered, `add-file "$key" "$filename"`)

	// Sanity: keystore creation and the initialized flag are still present.
	require.Contains(t, rendered, "/usr/share/elasticsearch/bin/elasticsearch-keystore create")
	require.Contains(t, rendered, "elastic-internal-init-keystore.ok")
}
