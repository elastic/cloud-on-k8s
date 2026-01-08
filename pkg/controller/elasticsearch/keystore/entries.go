// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"maps"
	"math/big"
	"sort"
)

// Bootstrap seed constants matching Elasticsearch's KeyStoreWrapper implementation.
const (
	// SeedSettingKey is the keystore setting name for the bootstrap seed.
	SeedSettingKey = "keystore.seed"
	// SeedLength is the length of the generated bootstrap seed.
	SeedLength = 20
	// SeedChars are the characters used to generate the bootstrap seed.
	SeedChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789~!@#$%^&*-_=+?"
)

// Entry represents a single keystore entry with a name and value.
type Entry struct {
	Name  string
	Value []byte
}

// Settings represents the secure settings to store in the keystore.
// Keys are setting names, values are the setting data.
type Settings map[string][]byte

// SerializeEntries serializes keystore entries into the encrypted payload format.
// The format matches Java's DataOutputStream format which is always big-endian:
//   - Entry count (4 bytes, big endian)
//   - For each entry:
//   - Name as modified UTF-8 (2-byte length prefix + UTF-8 bytes, like Java's writeUTF)
//   - Value length (4 bytes, big endian)
//   - Value (raw bytes)
//
// Note: This is the encrypted payload format, which is always big-endian regardless
// of keystore version. The outer framing (salt, IV, ciphertext lengths) uses
// version-specific endianness, but the encrypted content is always big-endian
// because Java's DataOutputStream is unconditionally big-endian.
func SerializeEntries(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer

	// Write entry count (always big endian, per DataOutputStream)
	if err := binary.Write(&buf, binary.BigEndian, int32(len(entries))); err != nil {
		return nil, fmt.Errorf("failed to write entry count: %w", err)
	}

	for _, e := range entries {
		// Write name using Java's modified UTF-8 format (writeUTF)
		if err := writeUTF(&buf, e.Name); err != nil {
			return nil, fmt.Errorf("failed to write name for %q: %w", e.Name, err)
		}

		// Write value length (always big endian)
		if err := binary.Write(&buf, binary.BigEndian, int32(len(e.Value))); err != nil {
			return nil, fmt.Errorf("failed to write value length for %q: %w", e.Name, err)
		}

		// Write value bytes
		if _, err := buf.Write(e.Value); err != nil {
			return nil, fmt.Errorf("failed to write value for %q: %w", e.Name, err)
		}
	}

	return buf.Bytes(), nil
}

// writeUTF writes a string in Java's modified UTF-8 format.
// Format: 2-byte unsigned short length (big endian) + UTF-8 encoded bytes.
// This matches Java's DataOutputStream.writeUTF().
func writeUTF(buf *bytes.Buffer, s string) error {
	// Java's writeUTF uses a 2-byte unsigned short for the length
	// which limits strings to 65535 bytes of UTF-8 data
	utf8Bytes := []byte(s)
	if len(utf8Bytes) > 65535 {
		return fmt.Errorf("string too long for modified UTF-8: %d bytes", len(utf8Bytes))
	}

	// Write 2-byte length prefix (big endian unsigned short)
	if err := binary.Write(buf, binary.BigEndian, uint16(len(utf8Bytes))); err != nil {
		return err
	}

	// Write the UTF-8 bytes
	_, err := buf.Write(utf8Bytes)
	return err
}

// SettingsToEntries converts a Settings map to a sorted slice of Entry.
// Sorting ensures deterministic output for the same input settings.
func SettingsToEntries(settings Settings) []Entry {
	entries := make([]Entry, 0, len(settings))
	for name, value := range settings {
		entries = append(entries, Entry{Name: name, Value: value})
	}
	// Sort by name for deterministic output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// GenerateBootstrapSeed creates the required keystore.seed entry.
// This seed may be used as a unique, secure, random value by the Elasticsearch node.
func GenerateBootstrapSeed() ([]byte, error) {
	seed := make([]byte, SeedLength)
	chars := []byte(SeedChars)
	charsLen := big.NewInt(int64(len(chars)))

	for i := 0; i < SeedLength; i++ {
		idx, err := rand.Int(rand.Reader, charsLen)
		if err != nil {
			return nil, fmt.Errorf("failed to generate random index: %w", err)
		}
		seed[i] = chars[idx.Int64()]
	}
	return seed, nil
}

// EnsureBootstrapSeed ensures the settings contain a bootstrap seed.
// If not present, a new one is generated and added.
// Returns the (possibly modified) settings.
func EnsureBootstrapSeed(settings Settings) (Settings, error) {
	if _, ok := settings[SeedSettingKey]; ok {
		return settings, nil
	}

	seed, err := GenerateBootstrapSeed()
	if err != nil {
		return nil, err
	}

	// Create a copy to avoid modifying the original
	result := make(Settings, len(settings)+1)
	maps.Copy(result, settings)
	result[SeedSettingKey] = seed
	return result, nil
}
