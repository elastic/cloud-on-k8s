// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apikey

import (
	"testing"
)

func Test_IsManagedByECK(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     bool
	}{
		{
			name:     "nil metadata returns false",
			metadata: nil,
			want:     false,
		},
		{
			name:     "missing managed-by metadata returns false",
			metadata: map[string]any{},
			want:     false,
		},
		{
			name: "wrong managed-by value returns false",
			metadata: map[string]any{
				MetadataKeyManagedBy: "wrong-value",
			},
			want: false,
		},
		{
			name: "correct managed-by value returns true",
			metadata: map[string]any{
				MetadataKeyManagedBy: MetadataValueECK,
			},
			want: true,
		},
		{
			name: "correct managed-by value with other metadata returns true",
			metadata: map[string]any{
				MetadataKeyManagedBy:  MetadataValueECK,
				MetadataKeyConfigHash: "hash123",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsManagedByECK(tt.metadata); got != tt.want {
				t.Errorf("IsManagedByECK() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_NeedsUpdate(t *testing.T) {
	tests := []struct {
		name         string
		metadata     map[string]any
		expectedHash string
		want         bool
	}{
		{
			name:         "nil metadata requires update",
			metadata:     nil,
			expectedHash: "hash123",
			want:         true,
		},
		{
			name:         "missing config hash requires update",
			metadata:     map[string]any{},
			expectedHash: "hash123",
			want:         true,
		},
		{
			name: "missing config hash with managed-by requires update",
			metadata: map[string]any{
				MetadataKeyManagedBy: MetadataValueECK,
			},
			expectedHash: "hash123",
			want:         true,
		},
		{
			name: "wrong config hash requires update",
			metadata: map[string]any{
				MetadataKeyConfigHash: "wrong-hash",
			},
			expectedHash: "hash123",
			want:         true,
		},
		{
			name: "wrong config hash with managed-by requires update",
			metadata: map[string]any{
				MetadataKeyManagedBy:  MetadataValueECK,
				MetadataKeyConfigHash: "wrong-hash",
			},
			expectedHash: "hash123",
			want:         true,
		},
		{
			name: "correct config hash does not require update",
			metadata: map[string]any{
				MetadataKeyConfigHash: "hash123",
			},
			expectedHash: "hash123",
			want:         false,
		},
		{
			name: "correct config hash with managed-by does not require update",
			metadata: map[string]any{
				MetadataKeyManagedBy:  MetadataValueECK,
				MetadataKeyConfigHash: "hash123",
			},
			expectedHash: "hash123",
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsUpdate(tt.metadata, tt.expectedHash); got != tt.want {
				t.Errorf("NeedsUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
