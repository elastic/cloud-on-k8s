// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var testReplacement = Replacement{
	Path: []string{
		"node", "roles",
	},
	Expected:    nil,
	Replacement: make([]string, 0),
}

func TestCanonicalConfig_RenderSeq(t *testing.T) {

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected []byte
	}{
		{
			name: "nil roles replacement",
			input: map[string]interface{}{
				"node": map[string]interface{}{
					"roles": make([]string, 0),
				},
			},
			expected: []byte(`node:
  roles: []
`),
		},
		{
			name: "does not touch non-nil values",
			input: map[string]interface{}{
				"node": map[string]interface{}{
					"roles": []string{"master"},
				},
			},
			expected: []byte(`node:
  roles:
  - master
`),
		},
		{
			name: "does not touch non-nil values",
			input: map[string]interface{}{
				"node": map[string]interface{}{
					"something": "else",
				},
			},
			expected: []byte(`node:
  something: else
`),
		},
	}

	for _, tt := range tests {
		config := MustCanonicalConfig(tt.input)
		output, err := config.Render(testReplacement)
		require.NoError(t, err)
		require.Equal(t, tt.expected, output)

	}
}
