// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package main

import (
	"crypto/x509"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_privateKey(t *testing.T) {
	config := tmpConfig()
	defer cleanTmpConfig(config)
	// write
	createdPrivateKey, err := createAndStorePrivateKey(config.PrivateKeyPath)
	require.NoError(t, err)
	require.NoError(t, createdPrivateKey.Validate())
	// read
	readPrivateKey, err := readPrivateKey(config.PrivateKeyPath)
	require.NoError(t, err)
	// compare
	require.Equal(t, *createdPrivateKey.D, *readPrivateKey.D)
	require.EqualValues(t, createdPrivateKey.Primes, readPrivateKey.Primes)
}

func Test_CSR(t *testing.T) {
	config := tmpConfig()
	defer cleanTmpConfig(config)
	privateKey, err := createAndStorePrivateKey(config.PrivateKeyPath)
	require.NoError(t, err)
	// create
	csr, err := createCSR(privateKey)
	require.NoError(t, err)
	require.NotEmpty(t, csr)
	parsed, err := x509.ParseCertificateRequest(csr)
	require.NoError(t, err)
	require.NoError(t, parsed.CheckSignature())
	// write
	err = ioutil.WriteFile(config.CSRPath, csr, 644)
	require.NoError(t, err)
	// read
	readCSR, err := readCSR(config.CSRPath)
	require.NoError(t, err)
	// compare
	require.Equal(t, parsed, readCSR)
}
