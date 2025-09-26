// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_categorizeAllowedCharacters(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedOther   []rune
		expectedLower   string
		expectedUpper   string
		expectedDigits  string
		expectedSymbols string
	}{
		{
			name:            "spaces show up in other slice",
			input:           "abc XYZ 123 !@# \t\n",
			expectedOther:   []rune{' ', ' ', ' ', ' ', '\t', '\n'},
			expectedLower:   "abc",
			expectedUpper:   "XYZ",
			expectedDigits:  "123",
			expectedSymbols: "!@#",
		},
		{
			name:            "unicode emojis show up in other slice",
			input:           "abc123😀🎉👍",
			expectedOther:   []rune{'😀', '🎉', '👍'},
			expectedLower:   "abc",
			expectedUpper:   "",
			expectedDigits:  "123",
			expectedSymbols: "",
		},
		{
			name:            "various unicode characters in other slice",
			input:           "café123ñΩ€",
			expectedOther:   []rune{'é', 'ñ', 'Ω', '€'},
			expectedLower:   "caf",
			expectedUpper:   "",
			expectedDigits:  "123",
			expectedSymbols: "",
		},
		{
			name:            "password constants do not show up in other slice",
			input:           LowerLetters + UpperLetters + Digits + Symbols,
			expectedOther:   nil,
			expectedLower:   LowerLetters,
			expectedUpper:   UpperLetters,
			expectedDigits:  Digits,
			expectedSymbols: Symbols,
		},
		{
			name:            "empty string",
			input:           "",
			expectedOther:   nil,
			expectedLower:   "",
			expectedUpper:   "",
			expectedDigits:  "",
			expectedSymbols: "",
		},
		{
			name:            "only spaces and tabs",
			input:           "   \t\t   ",
			expectedOther:   []rune{' ', ' ', ' ', '\t', '\t', ' ', ' ', ' '},
			expectedLower:   "",
			expectedUpper:   "",
			expectedDigits:  "",
			expectedSymbols: "",
		},
		{
			name:            "control characters in other slice",
			input:           "abc\x00\x01\x02",
			expectedOther:   []rune{'\x00', '\x01', '\x02'},
			expectedLower:   "abc",
			expectedUpper:   "",
			expectedDigits:  "",
			expectedSymbols: "",
		},
		{
			name:            "non-latin scripts in other slice",
			input:           "abc123αβγ中文العربية",
			expectedOther:   []rune{'α', 'β', 'γ', '中', '文', 'ا', 'ل', 'ع', 'ر', 'ب', 'ي', 'ة'},
			expectedLower:   "abc",
			expectedUpper:   "",
			expectedDigits:  "123",
			expectedSymbols: "",
		},
		{
			name:            "mathematical symbols some in symbols some in other",
			input:           "+-*/=<>≠≤≥∑∏",
			expectedOther:   []rune{'≠', '≤', '≥', '∑', '∏'},
			expectedLower:   "",
			expectedUpper:   "",
			expectedDigits:  "",
			expectedSymbols: "+-*/=<>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, other := categorizeAllowedCharacters(tt.input)

			// Check that spaces, emojis, and unicode chars are in other slice
			require.Equal(t, tt.expectedOther, other, "other slice should contain expected characters")

			// Check that character constants are properly categorized
			require.Equal(t, tt.expectedLower, params.LowerLetters, "lowercase letters should match expected")
			require.Equal(t, tt.expectedUpper, params.UpperLetters, "uppercase letters should match expected")
			require.Equal(t, tt.expectedDigits, params.Digits, "digits should match expected")
			require.Equal(t, tt.expectedSymbols, params.Symbols, "symbols should match expected")

			// Verify that none of the defined constant characters appear in other
			for _, r := range other {
				require.NotContains(t, LowerLetters, string(r), "lowercase letter should not be in other slice")
				require.NotContains(t, UpperLetters, string(r), "uppercase letter should not be in other slice")
				require.NotContains(t, Digits, string(r), "digit should not be in other slice")
				require.NotContains(t, Symbols, string(r), "symbol should not be in other slice")
			}
		})
	}
}

func Test_validateCharactersInParams(t *testing.T) {
	tests := []struct {
		name        string
		params      GeneratorParams
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid params with all default constants",
			params: GeneratorParams{
				LowerLetters: LowerLetters,
				UpperLetters: UpperLetters,
				Digits:       Digits,
				Symbols:      Symbols,
				Length:       24,
			},
			expectError: false,
		},
		{
			name: "valid params with subset of constants",
			params: GeneratorParams{
				LowerLetters: "abc",
				UpperLetters: "XYZ",
				Digits:       "123",
				Symbols:      "!@#",
				Length:       12,
			},
			expectError: false,
		},
		{
			name: "invalid lowercase letter",
			params: GeneratorParams{
				LowerLetters: "abcé",
				UpperLetters: "ABC",
				Digits:       "123",
				Symbols:      "!@#",
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character 'é' in LowerLetters",
		},
		{
			name: "invalid uppercase letter",
			params: GeneratorParams{
				LowerLetters: "abc",
				UpperLetters: "ABCΩ",
				Digits:       "123",
				Symbols:      "!@#",
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character 'Ω' in UpperLetters",
		},
		{
			name: "invalid digit",
			params: GeneratorParams{
				LowerLetters: "abc",
				UpperLetters: "ABC",
				Digits:       "123A",
				Symbols:      "!@#",
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character 'A' in Digits",
		},
		{
			name: "invalid symbol",
			params: GeneratorParams{
				LowerLetters: "abc",
				UpperLetters: "ABC",
				Digits:       "123",
				Symbols:      "!@#α", // α is not in Symbols constant
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character 'α' in Symbols",
		},
		{
			name: "emoji in symbols",
			params: GeneratorParams{
				LowerLetters: "abc",
				UpperLetters: "ABC",
				Digits:       "123",
				Symbols:      "!@#😀",
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character '😀' in Symbols",
		},
		{
			name: "space character in lowercase",
			params: GeneratorParams{
				LowerLetters: "abc ",
				UpperLetters: "ABC",
				Digits:       "123",
				Symbols:      "!@#",
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character ' ' in LowerLetters",
		},
		{
			name: "tab character in digits",
			params: GeneratorParams{
				LowerLetters: "abc",
				UpperLetters: "ABC",
				Digits:       "123\t",
				Symbols:      "!@#",
				Length:       10,
			},
			expectError: true,
			errorMsg:    "invalid character '\\t' in Digits",
		},
		{
			name: "all fields empty",
			params: GeneratorParams{
				LowerLetters: "",
				UpperLetters: "",
				Digits:       "",
				Symbols:      "",
				Length:       10,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCharactersInParams(tt.params)

			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, tt.errorMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
