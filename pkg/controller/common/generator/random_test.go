// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package generator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomBytes(t *testing.T) {
	tests := []struct {
		name     string
		params   ByteGeneratorParams
		wantErr  bool
		validate func(t *testing.T, result []byte, params ByteGeneratorParams)
	}{
		{
			name: "basic password generation",
			params: ByteGeneratorParams{
				LowerLetters: "abcdefghijklmnopqrstuvwxyz",
				UpperLetters: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Digits:       "0123456789",
				Symbols:      "!@#$%^&*",
				Length:       16,
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				assert.Len(t, password, params.Length)

				counts := countCharacterTypes(password, params)

				// Verify we have the expected number of digits (4 [16/4])
				expectedDigits := 4
				assert.Equal(t, expectedDigits, counts.digits, "Expected %d digits, got %d", expectedDigits, counts.digits)

				// Verify we have the expected number of symbols (4)
				expectedSymbols := 4
				assert.Equal(t, expectedSymbols, counts.symbols, "Expected %d symbols, got %d", expectedSymbols, counts.symbols)

				// Verify all characters are from the allowed sets
				for _, char := range password {
					assert.True(t, strings.ContainsRune(params.LowerLetters+params.UpperLetters+params.Digits+params.Symbols, char),
						"Character '%c' not found in allowed character sets", char)
				}
			},
		},
		{
			name: "long password with better distribution",
			params: ByteGeneratorParams{
				LowerLetters: "abcdefghijklmnopqrstuvwxyz",
				UpperLetters: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Digits:       "0123456789",
				Symbols:      "!@#$%^&*()-_=+[]{}|;:,.<>?",
				Length:       32,
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				assert.Len(t, password, params.Length)

				counts := countCharacterTypes(password, params)

				// For length 32: expected 8 digits and 8 symbols
				expectedDigits := 8
				expectedSymbols := 8

				assert.Equal(t, expectedDigits, counts.digits)
				assert.Equal(t, expectedSymbols, counts.symbols)

				// Should have some uppercase and lowercase letters
				assert.Greater(t, counts.upper, 0, "Should have at least one uppercase letter")
				assert.Greater(t, counts.lower, 0, "Should have at least one lowercase letter")
			},
		},
		{
			name: "limited digits available",
			params: ByteGeneratorParams{
				LowerLetters: "abcdefghijklmnopqrstuvwxyz",
				UpperLetters: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Digits:       "01", // Only 2 digits available
				Symbols:      "!@#$%^&*",
				Length:       16,
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				counts := countCharacterTypes(password, params)

				// Should only have 2 digits (limited by available digits, not length/4)
				expectedDigits := 2
				assert.Equal(t, expectedDigits, counts.digits)
			},
		},
		{
			name: "limited symbols available",
			params: ByteGeneratorParams{
				LowerLetters: "abcdefghijklmnopqrstuvwxyz",
				UpperLetters: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Digits:       "0123456789",
				Symbols:      "!", // Only 1 symbol available
				Length:       16,
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				counts := countCharacterTypes(password, params)

				// Should only have 1 symbol (limited by available symbols, not length/4)
				expectedSymbols := 1
				assert.Equal(t, expectedSymbols, counts.symbols)
			},
		},
		{
			name: "no digits or symbols",
			params: ByteGeneratorParams{
				LowerLetters: "abcdefghijklmnopqrstuvwxyz",
				UpperLetters: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Digits:       "",
				Symbols:      "",
				Length:       16,
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				counts := countCharacterTypes(password, params)

				assert.Equal(t, 0, counts.digits, "Should have no digits")
				assert.Equal(t, 0, counts.symbols, "Should have no symbols")
				assert.Equal(t, 16, counts.lower+counts.upper, "Password length should be 16")
			},
		},
		{
			name: "short password splits all digits and symbols",
			params: ByteGeneratorParams{
				LowerLetters: "abcdefghijklmnopqrstuvwxyz",
				UpperLetters: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Digits:       "0123456789",
				Symbols:      "!@#$%^&*",
				Length:       4,
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				assert.Len(t, password, params.Length)

				counts := countCharacterTypes(password, params)

				// For length 4: expected 1 digit and 1 symbol (4/4 = 1)
				expectedDigits := 1
				expectedSymbols := 1

				assert.Equal(t, expectedDigits, counts.digits)
				assert.Equal(t, expectedSymbols, counts.symbols)
			},
		},
		{
			name: "empty character set uses default charset",
			params: ByteGeneratorParams{
				LowerLetters: "",
				UpperLetters: "",
				Digits:       "",
				Symbols:      "",
				Length:       8,
			},
			wantErr: false, // This doesn't error, it uses default character set
			validate: func(t *testing.T, result []byte, params ByteGeneratorParams) {
				t.Helper()
				password := string(result)
				assert.Len(t, password, params.Length)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RandomBytes(tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.validate != nil {
				tt.validate(t, result, tt.params)
			}
		})
	}
}

// Helper function to count different character types in a password
type characterCounts struct {
	lower   int
	upper   int
	digits  int
	symbols int
}

func countCharacterTypes(password string, params ByteGeneratorParams) characterCounts {
	counts := characterCounts{}

	for _, char := range password {
		switch {
		case strings.ContainsRune(params.LowerLetters, char):
			counts.lower++
		case strings.ContainsRune(params.UpperLetters, char):
			counts.upper++
		case strings.ContainsRune(params.Digits, char):
			counts.digits++
		case strings.ContainsRune(params.Symbols, char):
			counts.symbols++
		}
	}

	return counts
}
