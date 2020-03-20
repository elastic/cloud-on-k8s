// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package dependency

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOverrides(t *testing.T) {
	overrides, err := LoadOverrides("testdata/overrides.json")
	require.NoError(t, err)
	require.Len(t, overrides, 4)

	o1 := overrides["my.pkg/v1"]
	require.Equal(t, "Apache-2.0", o1.LicenceType)
	require.Empty(t, o1.URL)

	o2 := overrides["my.otherpkg/v1"]
	require.Equal(t, "https://me.example.com/pkg", o2.URL)
	require.Empty(t, o2.LicenceType)

	o3LicencePath, err := filepath.Abs("./testdata/my/securepkg/v1/licence.txt")
	require.NoError(t, err)
	o3 := overrides["my.securepkg/v1"]
	require.Equal(t, o3LicencePath, o3.LicenceFile)
	require.Empty(t, o3.LicenceType)

	o4LicencePath, err := filepath.Abs("./testdata/etc/passwd")
	require.NoError(t, err)
	o4 := overrides["my.insecurepkg/v1"]
	require.Equal(t, o4LicencePath, o4.LicenceFile)
	require.Empty(t, o4.LicenceType)
}
