// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
)

// TestKeystoreParams_BatchedAddFile renders the keystore init script using the
// shared production helper and asserts the rendered output matches the golden
// snapshot. The snapshot guards against any structural change (whitespace,
// ordering, command shape) that would silently regress the per-pod startup
// cost win from elastic/cloud-on-k8s#9439.
func TestKeystoreParams_BatchedAddFile(t *testing.T) {
	require.Equal(t, keystoreScript, KeystoreParams.CustomScript)

	rendered, err := keystore.RenderInitScript(KeystoreParams)
	require.NoError(t, err)

	snaps.MatchSnapshot(t, rendered)
}
