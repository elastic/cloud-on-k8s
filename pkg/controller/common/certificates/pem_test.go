// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package certificates

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_PrivateMatchesPublicKey(t *testing.T) {
	rsaPrivateKey1, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	rsaPrivateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	ecdsaPrivateKey1, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)
	ecdsaPrivateKey2, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)
	tests := []struct {
		name       string
		publicKey  crypto.PublicKey
		privateKey crypto.Signer
		want       bool
	}{
		{
			name:       "with matching RSA public and private keys",
			publicKey:  rsaPrivateKey1.Public(),
			privateKey: rsaPrivateKey1,
			want:       true,
		},
		{
			name:       "with non-matching RSA public and private keys",
			publicKey:  rsaPrivateKey1.Public(),
			privateKey: rsaPrivateKey2,
			want:       false,
		},
		{
			name:       "with matching ECDSA public and private keys",
			publicKey:  ecdsaPrivateKey1.Public(),
			privateKey: ecdsaPrivateKey1,
			want:       true,
		},
		{
			name:       "with non-matching ECDSA public and private keys",
			publicKey:  ecdsaPrivateKey1.Public(),
			privateKey: ecdsaPrivateKey2,
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrivateMatchesPublicKey(context.Background(), tt.publicKey, tt.privateKey); got != tt.want {
				t.Errorf("privateMatchesPublicKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
