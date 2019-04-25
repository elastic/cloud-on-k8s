// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package main

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_generateSrc(t *testing.T) {
	path := filepath.Join("testdata", "expected.src")
	expectedBytes, err := ioutil.ReadFile(path)
	require.NoError(t, err)
	input := filepath.Join("testdata", "test.key")
	var out bytes.Buffer
	generateSrc(input, &out)
	require.Equal(t, expectedBytes, out.Bytes())
}
