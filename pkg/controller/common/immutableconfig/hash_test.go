// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeContentHash(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string][]byte
		wantLen  int
		wantSame bool // if true, verify determinism by computing twice
	}{
		{
			name:     "empty map",
			data:     map[string][]byte{},
			wantLen:  64, // SHA-256 hex is 64 chars
			wantSame: true,
		},
		{
			name: "single entry",
			data: map[string][]byte{
				"config.yml": []byte("key: value"),
			},
			wantLen:  64,
			wantSame: true,
		},
		{
			name: "multiple entries - order independent",
			data: map[string][]byte{
				"b.yml": []byte("b content"),
				"a.yml": []byte("a content"),
				"c.yml": []byte("c content"),
			},
			wantLen:  64,
			wantSame: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := ComputeContentHash(tt.data)
			assert.Len(t, hash1, tt.wantLen)

			if tt.wantSame {
				hash2 := ComputeContentHash(tt.data)
				assert.Equal(t, hash1, hash2, "hash should be deterministic")
			}
		})
	}
}

func TestComputeContentHash_OrderIndependent(t *testing.T) {
	data1 := map[string][]byte{
		"a.yml": []byte("a"),
		"b.yml": []byte("b"),
		"c.yml": []byte("c"),
	}
	data2 := map[string][]byte{
		"c.yml": []byte("c"),
		"a.yml": []byte("a"),
		"b.yml": []byte("b"),
	}

	hash1 := ComputeContentHash(data1)
	hash2 := ComputeContentHash(data2)
	assert.Equal(t, hash1, hash2, "hash should be independent of map iteration order")
}

func TestComputeContentHash_DifferentContent(t *testing.T) {
	data1 := map[string][]byte{"config.yml": []byte("value1")}
	data2 := map[string][]byte{"config.yml": []byte("value2")}

	hash1 := ComputeContentHash(data1)
	hash2 := ComputeContentHash(data2)
	assert.NotEqual(t, hash1, hash2, "different content should produce different hashes")
}

func TestComputeContentHash_DifferentKeys(t *testing.T) {
	data1 := map[string][]byte{"key1": []byte("value")}
	data2 := map[string][]byte{"key2": []byte("value")}

	hash1 := ComputeContentHash(data1)
	hash2 := ComputeContentHash(data2)
	assert.NotEqual(t, hash1, hash2, "different keys should produce different hashes")
}

func TestComputeStringContentHash(t *testing.T) {
	byteData := map[string][]byte{"config.yml": []byte("content")}
	stringData := map[string]string{"config.yml": "content"}

	byteHash := ComputeContentHash(byteData)
	stringHash := ComputeStringContentHash(stringData)
	assert.Equal(t, byteHash, stringHash, "string and byte versions should produce same hash")
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		name     string
		fullHash string
		n        int
		want     string
	}{
		{
			name:     "normal truncation",
			fullHash: "a1b2c3d4e5f6g7h8",
			n:        8,
			want:     "a1b2c3d4",
		},
		{
			name:     "hash shorter than n",
			fullHash: "abc",
			n:        8,
			want:     "abc",
		},
		{
			name:     "exact length",
			fullHash: "a1b2c3d4",
			n:        8,
			want:     "a1b2c3d4",
		},
		{
			name:     "zero length",
			fullHash: "a1b2c3d4",
			n:        0,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortHash(tt.fullHash, tt.n)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImmutableName(t *testing.T) {
	hash := ComputeContentHash(map[string][]byte{"key": []byte("value")})
	name := ImmutableName("my-config", hash)

	require.Contains(t, name, "my-config-")
	assert.Len(t, name, len("my-config-")+DefaultShortHashLen)
}
