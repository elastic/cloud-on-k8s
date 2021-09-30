// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package compare

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// JSONEqual compares the JSON representation of two objects to ensure they are equal.
func JSONEqual(t *testing.T, want, have interface{}) {
	t.Helper()

	w, err := json.Marshal(want)
	require.NoError(t, err)

	h, err := json.Marshal(have)
	require.NoError(t, err)

	require.JSONEq(t, string(w), string(h))
}
