// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_generateSrc(t *testing.T) {
	path := filepath.Join("testdata", "expected.src")
	expectedBytes, err := os.ReadFile(path)
	require.NoError(t, err)
	input := filepath.Join("testdata", "test.key")
	var out bytes.Buffer
	bytes, err := os.ReadFile(input)
	require.NoError(t, err)
	generateSrc(bytes, &out)
	require.Equal(t, expectedBytes, out.Bytes())
}
