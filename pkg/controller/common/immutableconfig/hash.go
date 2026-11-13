// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

const (
	// DefaultShortHashLen is 8 hex characters, providing ~4 billion possible values.
	DefaultShortHashLen = 8
)

// ComputeContentHash computes a deterministic content hash from the given data map.
// Keys are sorted lexicographically before hashing to ensure map iteration order does not affect
// the result. The full hex-encoded SHA-256 hash is returned.
func ComputeContentHash(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		// Write key, null separator, value for each entry.
		// The null byte prevents ambiguity between key/value boundaries
		// (e.g., key="ab" value="cd" vs key="a" value="bcd").
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write(data[k])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeStringContentHash computes a deterministic content hash from a string data map.
// This is a convenience wrapper for ConfigMap data which uses map[string]string.
func ComputeStringContentHash(data map[string]string) string {
	byteData := make(map[string][]byte, len(data))
	for k, v := range data {
		byteData[k] = []byte(v)
	}
	return ComputeContentHash(byteData)
}

// ShortHash returns the first n characters of the given full hex hash.
func ShortHash(fullHash string, n int) string {
	if len(fullHash) < n {
		return fullHash
	}
	return fullHash[:n]
}

// ImmutableName returns "{baseName}-{shortHash}" for content-addressed resource naming.
func ImmutableName(baseName, fullHash string) string {
	return fmt.Sprintf("%s-%s", baseName, ShortHash(fullHash, DefaultShortHashLen))
}
