// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	// Verify the V7 configuration values
	assert.Equal(t, 210000, Config.KDFIterations)
	assert.Equal(t, 256, Config.CipherKeyBits)
	assert.True(t, Config.UseLittleEndian)
}

func TestGenerateBootstrapSeed(t *testing.T) {
	seed1, err := GenerateBootstrapSeed()
	require.NoError(t, err)
	assert.Len(t, seed1, SeedLength)

	// Verify all characters are from the allowed set
	for _, b := range seed1 {
		assert.Contains(t, SeedChars, string(b))
	}

	// Verify randomness (two seeds should be different)
	seed2, err := GenerateBootstrapSeed()
	require.NoError(t, err)
	assert.NotEqual(t, seed1, seed2)
}

func TestEnsureBootstrapSeed(t *testing.T) {
	t.Run("adds seed when missing", func(t *testing.T) {
		settings := Settings{
			"some.setting": []byte("value"),
		}
		result, err := EnsureBootstrapSeed(settings)
		require.NoError(t, err)
		assert.Contains(t, result, SeedSettingKey)
		assert.Len(t, result[SeedSettingKey], SeedLength)
		// Original should not be modified
		assert.NotContains(t, settings, SeedSettingKey)
	})

	t.Run("preserves existing seed", func(t *testing.T) {
		existingSeed := []byte("my-custom-seed-value")
		settings := Settings{
			SeedSettingKey: existingSeed,
		}
		result, err := EnsureBootstrapSeed(settings)
		require.NoError(t, err)
		assert.Equal(t, existingSeed, result[SeedSettingKey])
	})
}

func TestSettingsToEntries(t *testing.T) {
	settings := Settings{
		"z.setting": []byte("z-value"),
		"a.setting": []byte("a-value"),
		"m.setting": []byte("m-value"),
	}

	entries := SettingsToEntries(settings)

	// Should be sorted alphabetically
	require.Len(t, entries, 3)
	assert.Equal(t, "a.setting", entries[0].Name)
	assert.Equal(t, "m.setting", entries[1].Name)
	assert.Equal(t, "z.setting", entries[2].Name)
}

func TestSerializeEntries(t *testing.T) {
	entries := []Entry{
		{Name: "key1", Value: []byte("value1")},
		{Name: "key2", Value: []byte("value2")},
	}

	// Encrypted payload always uses big-endian per Java's DataOutputStream
	data, err := SerializeEntries(entries)
	require.NoError(t, err)

	// Parse the serialized data
	buf := bytes.NewReader(data)

	// Read entry count (4 bytes, big endian)
	var count int32
	require.NoError(t, binary.Read(buf, binary.BigEndian, &count))
	assert.Equal(t, int32(2), count)

	// Read first entry name using Java's modified UTF-8 format
	// First: 2-byte length prefix (unsigned short, big endian)
	var nameLen uint16
	require.NoError(t, binary.Read(buf, binary.BigEndian, &nameLen))
	assert.Equal(t, uint16(4), nameLen) // "key1" is 4 bytes

	// Read the name bytes
	nameBytes := make([]byte, nameLen)
	_, err = buf.Read(nameBytes)
	require.NoError(t, err)
	assert.Equal(t, "key1", string(nameBytes))

	// Read value length (4 bytes, big endian)
	var valueLen int32
	require.NoError(t, binary.Read(buf, binary.BigEndian, &valueLen))
	assert.Equal(t, int32(6), valueLen) // "value1" is 6 bytes

	// Read value bytes
	valueBytes := make([]byte, valueLen)
	_, err = buf.Read(valueBytes)
	require.NoError(t, err)
	assert.Equal(t, "value1", string(valueBytes))
}

func TestEncrypt(t *testing.T) {
	key := make([]byte, 32) // 256-bit key
	iv := make([]byte, IVLength)
	aad := make([]byte, SaltLength) // AAD is the salt
	plaintext := []byte("secret data")

	ciphertext, err := Encrypt(plaintext, key, iv, aad)
	require.NoError(t, err)

	// Ciphertext should be longer than plaintext (includes GCM tag)
	assert.Greater(t, len(ciphertext), len(plaintext))

	// Encrypting same data with same key/iv/aad should give same result
	ciphertext2, err := Encrypt(plaintext, key, iv, aad)
	require.NoError(t, err)
	assert.Equal(t, ciphertext, ciphertext2)
}

func TestDeriveKey(t *testing.T) {
	password := []byte("test-password")
	salt := make([]byte, SaltLength)

	t.Run("produces 256-bit key", func(t *testing.T) {
		key := DeriveKey(password, salt, Config)
		assert.Len(t, key, 32) // 256 bits = 32 bytes
	})

	t.Run("same inputs produce same key", func(t *testing.T) {
		key1 := DeriveKey(password, salt, Config)
		key2 := DeriveKey(password, salt, Config)
		assert.Equal(t, key1, key2)
	})

	t.Run("different passwords produce different keys", func(t *testing.T) {
		key1 := DeriveKey([]byte("password1"), salt, Config)
		key2 := DeriveKey([]byte("password2"), salt, Config)
		assert.NotEqual(t, key1, key2)
	})
}

func TestWriteHeader(t *testing.T) {
	var buf bytes.Buffer
	w := newChecksumWriter(&buf)

	err := WriteHeader(w, int32(KeystoreVersion))
	require.NoError(t, err)

	data := buf.Bytes()

	// Check magic number (always big endian)
	magic := binary.BigEndian.Uint32(data[0:4])
	assert.Equal(t, CodecMagic, magic)

	// Check codec name follows
	// VInt for length, then string bytes
	// For "elasticsearch.keystore" (22 chars), VInt is 1 byte
	nameLen := int(data[4])
	assert.Equal(t, len(CodecName), nameLen)

	name := string(data[5 : 5+nameLen])
	assert.Equal(t, CodecName, name)

	// Check version (big endian)
	version := binary.BigEndian.Uint32(data[5+nameLen : 5+nameLen+4])
	assert.Equal(t, uint32(KeystoreVersion), version)
}

func TestCreate(t *testing.T) {
	settings := Settings{
		"xpack.notification.email.account.foo.smtp.secure_password": []byte("secret"),
	}

	data, err := Create(settings)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Verify header magic
	magic := binary.BigEndian.Uint32(data[0:4])
	assert.Equal(t, CodecMagic, magic)

	// Verify keystore version in header
	versionOffset := 4 + 1 + len(CodecName)
	version := binary.BigEndian.Uint32(data[versionOffset : versionOffset+4])
	assert.Equal(t, uint32(KeystoreVersion), version)

	// Verify minimum size (header + password flag + some data + footer)
	assert.Greater(t, len(data), FooterLength+10)
}

func TestCreate_BootstrapSeedAdded(t *testing.T) {
	// Create keystore without explicit bootstrap seed
	settings := Settings{
		"some.setting": []byte("value"),
	}

	data, err := Create(settings)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// The keystore should be created successfully, indicating bootstrap seed was added
	// (We can't easily verify the content is encrypted correctly without decrypting)
}

func TestCreate_EmptySettings(t *testing.T) {
	// Empty settings should still work (bootstrap seed will be added)
	settings := Settings{}

	data, err := Create(settings)
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestChecksumWriter(t *testing.T) {
	var buf bytes.Buffer
	w := newChecksumWriter(&buf)

	testData := []byte("hello world")
	n, err := w.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, int64(len(testData)), w.Size())

	// Checksum should be non-zero for non-empty data
	assert.NotEqual(t, uint32(0), w.Checksum())

	// Same data should produce same checksum
	var buf2 bytes.Buffer
	w2 := newChecksumWriter(&buf2)
	_, _ = w2.Write(testData)
	assert.Equal(t, w.Checksum(), w2.Checksum())
}

func TestWriteVInt(t *testing.T) {
	tests := []struct {
		value    int32
		expected []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7F}},
		{128, []byte{0x80, 0x01}},
		{16383, []byte{0xFF, 0x7F}},
		{16384, []byte{0x80, 0x80, 0x01}},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		err := writeVInt(&buf, tt.value)
		require.NoError(t, err)
		assert.Equal(t, tt.expected, buf.Bytes(), "value: %d", tt.value)
	}
}

func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	require.NoError(t, err)
	assert.Len(t, salt, SaltLength)

	// Two salts should be different
	salt2, err := GenerateSalt()
	require.NoError(t, err)
	assert.NotEqual(t, salt, salt2)
}

func TestGenerateIV(t *testing.T) {
	iv, err := GenerateIV()
	require.NoError(t, err)
	assert.Len(t, iv, IVLength)

	// Two IVs should be different
	iv2, err := GenerateIV()
	require.NoError(t, err)
	assert.NotEqual(t, iv, iv2)
}
