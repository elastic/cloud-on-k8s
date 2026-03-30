// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAESECBRoundTrip(t *testing.T) {
	// Use a real DER-encoded public key as plaintext, mirroring actual usage in Sign.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	require.NoError(t, err)

	ciphertext, err := encryptWithAESECB(pubKeyBytes)
	require.NoError(t, err)

	decrypted, err := decryptWithAESECB(ciphertext)
	require.NoError(t, err)

	assert.Equal(t, pubKeyBytes, decrypted)
}

func TestDecryptWithAESECB_InvalidInput(t *testing.T) {
	_, err := decryptWithAESECB(nil)
	require.Error(t, err)

	_, err = decryptWithAESECB([]byte{1, 2, 3}) // not a multiple of block size
	require.Error(t, err)
}
