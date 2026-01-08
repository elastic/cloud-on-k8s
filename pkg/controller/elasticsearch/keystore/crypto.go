// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// Cryptographic constants matching Elasticsearch's KeyStoreWrapper implementation.
const (
	// SaltLength is the recommended salt size per OWASP guidelines (64 bytes).
	SaltLength = 64
	// IVLength is the IV size for AES-GCM as recommended by NIST SP 800-38D (12 bytes / 96 bits).
	IVLength = 12
	// GCMTagBits is the authentication tag size for AES-GCM (128 bits).
	GCMTagBits = 128
)

// DeriveKey derives an encryption key from a password using PBKDF2-HMAC-SHA512.
// This matches Elasticsearch's key derivation in KeyStoreWrapper.java.
func DeriveKey(password, salt []byte, config VersionConfig) []byte {
	keyBytes := config.CipherKeyBits / 8
	return pbkdf2.Key(password, salt, config.KDFIterations, keyBytes, sha512.New)
}

// Encrypt encrypts plaintext using AES-GCM with the provided key, IV, and AAD (Additional Authenticated Data).
// The salt is used as AAD to match Elasticsearch's keystore format.
// Returns the ciphertext with the GCM authentication tag appended.
func Encrypt(plaintext, key, iv, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(iv) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid IV length: got %d, want %d", len(iv), gcm.NonceSize())
	}

	// Seal appends the ciphertext and authentication tag to dst
	// The aad (salt) is used as additional authenticated data, matching Elasticsearch's cipher.updateAAD(salt)
	ciphertext := gcm.Seal(nil, iv, plaintext, aad)
	return ciphertext, nil
}

// GenerateSalt generates a cryptographically secure random salt.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// GenerateIV generates a cryptographically secure random initialization vector.
func GenerateIV() ([]byte, error) {
	iv := make([]byte, IVLength)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}
	return iv, nil
}
