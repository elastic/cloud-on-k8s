// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package keystore provides functionality to create Elasticsearch keystore files
// in Go, matching the format used by Elasticsearch's KeyStoreWrapper.
//
// This implementation supports ES 9.3+ which uses keystore format V7.
// The keystore format uses Lucene's codec utilities for file structure and
// AES-GCM encryption with PBKDF2 key derivation for securing the contents.
//
// See: https://github.com/elastic/elasticsearch/blob/main/server/src/main/java/org/elasticsearch/common/settings/KeyStoreWrapper.java
package keystore

import (
	"bytes"
	"fmt"
)

// Create generates an Elasticsearch keystore file containing the given settings.
// The keystore is created with an empty password (ECK-managed keystores don't use passwords).
//
// Parameters:
//   - settings: Map of setting names to their values
//
// Returns the keystore file contents as bytes, or an error if creation fails.
func Create(settings Settings) ([]byte, error) {
	// Ensure bootstrap seed is present
	settings, err := EnsureBootstrapSeed(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure bootstrap seed: %w", err)
	}

	// Serialize entries into the encrypted payload format (always big-endian per Java's DataOutputStream)
	entries := SettingsToEntries(settings)
	plaintext, err := SerializeEntries(entries)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize entries: %w", err)
	}

	// Generate cryptographic material
	salt, err := GenerateSalt()
	if err != nil {
		return nil, err
	}
	iv, err := GenerateIV()
	if err != nil {
		return nil, err
	}

	// Derive encryption key from empty password (ECK-managed keystores don't use passwords)
	key := DeriveKey([]byte{}, salt, Config)

	// Encrypt the serialized entries
	// The salt is used as AAD (Additional Authenticated Data) to match Elasticsearch's implementation
	ciphertext, err := Encrypt(plaintext, key, iv, salt)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt keystore: %w", err)
	}

	// Build the keystore file
	return buildKeystoreFile(salt, iv, ciphertext)
}

// buildKeystoreFile assembles the complete keystore file with header, data, and footer.
func buildKeystoreFile(salt, iv, ciphertext []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := newChecksumWriter(&buf)

	// Write Lucene codec header
	if err := WriteHeader(w, int32(KeystoreVersion)); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	// Write hasPassword flag (0x00 for no password, 0x01 for password)
	// ECK-managed keystores always use empty password
	if err := w.WriteByte(0x00); err != nil {
		return nil, fmt.Errorf("failed to write password flag: %w", err)
	}

	// Calculate and write the total data block size
	// Size = 4 (salt length) + salt + 4 (iv length) + iv + 4 (ciphertext length) + ciphertext
	dataSize := 4 + len(salt) + 4 + len(iv) + 4 + len(ciphertext)
	if err := writeInt(w, dataSize, Config.UseLittleEndian); err != nil {
		return nil, fmt.Errorf("failed to write data size: %w", err)
	}

	// Write salt
	if err := writeBytes(w, salt, Config.UseLittleEndian); err != nil {
		return nil, fmt.Errorf("failed to write salt: %w", err)
	}

	// Write IV
	if err := writeBytes(w, iv, Config.UseLittleEndian); err != nil {
		return nil, fmt.Errorf("failed to write IV: %w", err)
	}

	// Write encrypted data
	if err := writeBytes(w, ciphertext, Config.UseLittleEndian); err != nil {
		return nil, fmt.Errorf("failed to write ciphertext: %w", err)
	}

	// Write Lucene codec footer with checksum
	if err := WriteFooter(w); err != nil {
		return nil, fmt.Errorf("failed to write footer: %w", err)
	}

	return buf.Bytes(), nil
}
