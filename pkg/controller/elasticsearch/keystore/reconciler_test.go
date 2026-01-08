// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

func TestKeystoreSecretName(t *testing.T) {
	name := esv1.KeystoreSecretName("my-cluster")
	assert.Equal(t, "my-cluster-es-keystore", name)
}

func TestComputeSettingsHash(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
	}{
		{
			name:     "empty settings",
			settings: Settings{},
		},
		{
			name: "single setting",
			settings: Settings{
				"key1": []byte("value1"),
			},
		},
		{
			name: "multiple settings",
			settings: Settings{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
				"key3": []byte("value3"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := computeSettingsHash(tt.settings)
			hash2 := computeSettingsHash(tt.settings)

			// Hash should be deterministic
			assert.Equal(t, hash1, hash2)
			// Hash should be non-empty
			assert.NotEmpty(t, hash1)
			// Hash should be hex-encoded SHA-256 (64 characters)
			assert.Len(t, hash1, 64)
		})
	}

	t.Run("different settings produce different hashes", func(t *testing.T) {
		settings1 := Settings{"key1": []byte("value1")}
		settings2 := Settings{"key1": []byte("value2")}
		settings3 := Settings{"key2": []byte("value1")}

		hash1 := computeSettingsHash(settings1)
		hash2 := computeSettingsHash(settings2)
		hash3 := computeSettingsHash(settings3)

		assert.NotEqual(t, hash1, hash2, "different values should produce different hashes")
		assert.NotEqual(t, hash1, hash3, "different keys should produce different hashes")
	})

	t.Run("order independence", func(t *testing.T) {
		// Create settings in different orders
		settings1 := Settings{
			"aaa": []byte("1"),
			"bbb": []byte("2"),
			"ccc": []byte("3"),
		}
		settings2 := Settings{
			"ccc": []byte("3"),
			"aaa": []byte("1"),
			"bbb": []byte("2"),
		}

		hash1 := computeSettingsHash(settings1)
		hash2 := computeSettingsHash(settings2)

		assert.Equal(t, hash1, hash2, "order should not affect hash")
	})
}
