// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package certificates

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_PrivateMatchesPublicKey(t *testing.T) {
	privateKey1, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	tests := []struct {
		name       string
		publicKey  interface{}
		privateKey rsa.PrivateKey
		want       bool
	}{
		{
			name:       "with matching public and private keys",
			publicKey:  privateKey1.Public(),
			privateKey: *privateKey1,
			want:       true,
		},
		{
			name:       "with non-matching public and private keys",
			publicKey:  privateKey1.Public(),
			privateKey: *privateKey2,
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrivateMatchesPublicKey(tt.publicKey, tt.privateKey); got != tt.want {
				t.Errorf("privateMatchesPublicKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
