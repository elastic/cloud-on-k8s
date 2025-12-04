// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

func TestRandomPasswordGenerator_Generate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		length      int
		expectError bool
	}{
		{
			name:        "valid length 6",
			length:      6,
			expectError: false,
		},
		{
			name:        "valid length 24",
			length:      24,
			expectError: false,
		},
		{
			name:        "valid length 72",
			length:      72,
			expectError: false,
		},
		{
			name:        "length too small",
			length:      5,
			expectError: true,
		},
		{
			name:        "length too large",
			length:      73,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useLength := func(context.Context) (bool, error) { return true, nil }
			generator, err := NewRandomPasswordGenerator(tt.length, useLength)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, generator)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, generator)

			// Generate password multiple times to test randomness
			passwords := make([]string, 10)
			for i := range 10 {
				result, err := generator.Generate(ctx)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, result, tt.length, "Generated password should have expected length")

				passwords[i] = string(result)

				// Validate that all characters in the result are from the expected character set
				expectedCharSet := make(set.StringSet)
				for _, char := range defaultCharacterSet {
					expectedCharSet.Add(string(char))
				}

				for _, b := range result {
					require.True(t, expectedCharSet.Has(string(b)),
						"Character %s is not in expected character set %q",
						string(b), defaultCharacterSet)
				}
			}
		})
	}
}

func TestValidateLength(t *testing.T) {
	tests := []struct {
		name        string
		length      int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid minimum length",
			length:      6,
			expectError: false,
		},
		{
			name:        "valid maximum length",
			length:      72,
			expectError: false,
		},
		{
			name:        "valid middle length",
			length:      24,
			expectError: false,
		},
		{
			name:        "length too small",
			length:      5,
			expectError: true,
			errorMsg:    "password length must be at least 6 and at most 72",
		},
		{
			name:        "length too large",
			length:      73,
			expectError: true,
			errorMsg:    "password length must be at least 6 and at most 72",
		},
		{
			name:        "zero length",
			length:      0,
			expectError: true,
			errorMsg:    "password length must be at least 6 and at most 72",
		},
		{
			name:        "negative length",
			length:      -1,
			expectError: true,
			errorMsg:    "password length must be at least 6 and at most 72",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLength(tt.length)

			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, tt.errorMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
