// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helper

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPKCS8KeyEndsWithWhitespaceByte(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lastByte byte
		want     bool
	}{
		{"NUL", 0x00, true},
		{"tab", '\t', true},
		{"newline", '\n', true},
		{"vertical tab", '\v', true},
		{"form feed", '\f', true},
		{"carriage return", '\r', true},
		{"space", ' ', true},
		{"non-whitespace 0x01", 0x01, false},
		{"non-whitespace 0x21", 0x21, false},
		{"non-whitespace 0xff", 0xff, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keyPEM := pkcs8PEMWithLastByte(t, tc.lastByte)
			got, err := PKCS8KeyEndsWithWhitespaceByte(keyPEM)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPKCS8KeyEndsWithWhitespaceByte_InvalidPEM(t *testing.T) {
	_, err := PKCS8KeyEndsWithWhitespaceByte([]byte("not a pem block"))
	require.Error(t, err)
}

// pkcs8PEMWithLastByte generates a real PKCS#8 DER key and overwrites its last byte with b.
func pkcs8PEMWithLastByte(t *testing.T, b byte) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	der[len(der)-1] = b
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}
